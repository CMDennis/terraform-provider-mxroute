package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand/v2"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

type responseMode uint8

const (
	wrapped responseMode = iota
	raw
	sparse
)

type operation struct {
	name, method, path string
	status             int
	mode               responseMode
	reseller, shared   bool
}

func jsonShape(data json.RawMessage, t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) || len(data) == 0 {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil || fields == nil {
			return false
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			v, present := fields[strings.Split(f.Tag.Get("json"), ",")[0]]
			if !present {
				if f.Tag.Get("optional") == "true" {
					continue
				}
				return false
			}
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) && f.Tag.Get("nullable") == "true" {
				continue
			}
			if f.Tag.Get("optional") == "true" && bytes.Equal(bytes.TrimSpace(v), []byte(`""`)) {
				continue
			}
			if !jsonShape(v, f.Type) {
				return false
			}
		}
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(data, &items) != nil || items == nil {
			return false
		}
		for _, v := range items {
			if !jsonShape(v, t.Elem()) {
				return false
			}
		}
	case reflect.String:
		var v string
		if json.Unmarshal(data, &v) != nil || v == "" {
			return false
		}
	default:
		v := reflect.New(t).Interface()
		if json.Unmarshal(data, v) != nil {
			return false
		}
	}
	return true
}
func decode(data []byte, mode responseMode, out any) bool {
	if mode == raw {
		// The published quota schemas are raw objects, but live ordinary-account
		// responses also use the standard success/data envelope. Accept either
		// complete shape. An envelope marker must never fall back to raw decoding.
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			return false
		}
		if _, present := fields["success"]; present {
			mode = wrapped
		}
	}
	if mode == wrapped {
		var envelope struct {
			Success *bool           `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		if json.Unmarshal(data, &envelope) != nil || envelope.Success == nil || !*envelope.Success {
			return false
		}
		data = envelope.Data
	}
	return jsonShape(data, reflect.TypeOf(out).Elem()) && json.Unmarshal(data, out) == nil
}

// errorEnvelope never retains arbitrary server text. Even successful statuses
// are inspected before sparse or raw response handling.
func errorEnvelope(data []byte) (bool, string) {
	var env struct {
		Success *bool           `json:"success"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &env) != nil {
		return false, ""
	}
	var detail struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(env.Error, &detail)
	return env.Success != nil && !*env.Success, safeCode(detail.Code)
}
func backoff(base time.Duration, attempt int) time.Duration {
	ceiling := min(base*time.Duration(1<<attempt), 5*time.Second)
	if ceiling <= 0 {
		return 0
	}
	return ceiling/2 + time.Duration(rand.Int64N(int64(ceiling/2)+1))
}
func retryTime(h http.Header, now time.Time) time.Time {
	var until time.Time
	if value := h.Get("Retry-After"); value != "" {
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
			// Saturate rather than overflowing a malicious duration.
			until = now.Add(time.Duration(min(seconds, int64(24*60*60))) * time.Second)
		} else if date, err := http.ParseTime(value); err == nil {
			until = date
		}
	}
	if seconds, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil && seconds > 0 {
		if reset := time.Unix(seconds, 0); reset.After(until) {
			until = reset
		}
	}
	return until
}
func (c *Client) execute(ctx context.Context, op operation, body any, out any) error {
	if op.reseller {
		if err := c.RequireReseller(); err != nil {
			return &Error{Operation: op.name, Kind: Authorization}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	if op.shared && ctx.Value(sharedContextKey{}) != c.settings {
		if err := c.settings.acquire(ctx); err != nil {
			return err
		}
		defer c.settings.release()
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return &Error{Operation: op.name, Kind: Validation}
		}
		if strings.Contains(op.path, "/spam/") && len(payload) > 2048 {
			return &Error{Operation: op.name, Kind: Validation}
		}
	}
	read := op.method == http.MethodGet
	for attempt := 0; attempt < 3; attempt++ {
		if err := c.limits.reserve(ctx, read); err != nil {
			return err
		}
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(payload)
		}
		resp, requestErr := c.Request(ctx, op.method, op.path, reader)
		if requestErr != nil {
			if read && ctx.Err() == nil && attempt < 2 {
				if err := pause(ctx, backoff(c.retryBase, attempt)); err != nil {
					return err
				}
				continue
			}
			return &Error{Operation: op.name, Kind: Transport, Uncertain: !read, cause: requestErr}
		}
		data, bodyErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		_ = resp.Body.Close()
		now := time.Now()
		until := retryTime(resp.Header, now)
		if resp.StatusCode == 429 || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			if until.IsZero() {
				until = now.Add(time.Minute)
			}
			c.limits.block(read, until)
		}
		failed, code := errorEnvelope(data)
		e := &Error{Operation: op.name, Status: resp.StatusCode, Code: code, Kind: classify(resp.StatusCode, code)}
		if resp.StatusCode == 400 && (op.name == "CreateEmailAccount" || op.name == "UpdateEmailAccount") {
			e.passwordIssue = passwordRejection(data)
		}
		if bodyErr != nil || len(data) > maxResponseBytes {
			e.Kind, e.Uncertain = Protocol, !read
			// Cancellation can occur after headers arrive, while the body is
			// still streaming. Preserve it without retaining body error text.
			e.cause = ctx.Err()
			return e
		}
		if resp.StatusCode != op.status || failed {
			// Only HTTP 429 proves a write was rejected before processing. A 2xx
			// RATE_LIMITED envelope or 5xx can follow partial application.
			retry := resp.StatusCode == 429 || (read && resp.StatusCode >= 500)
			if retry && attempt < 2 {
				delay := backoff(c.retryBase, attempt)
				if until.After(now) {
					delay = max(delay, time.Until(until))
				}
				if err := pause(ctx, delay); err != nil {
					e.cause = err
					return e
				}
				continue
			}
			e.Uncertain = !read && (resp.StatusCode >= 500 || resp.StatusCode == op.status || (resp.StatusCode >= 200 && resp.StatusCode < 300))
			return e
		}
		if op.mode == sparse {
			return nil
		}
		contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil || (contentType != "application/json" && !strings.HasSuffix(contentType, "+json")) || !decode(data, op.mode, out) {
			e.Kind, e.Uncertain = Protocol, !read
			return e
		}
		return nil
	}
	panic("bounded request loop exhausted")
}

// Categorize the documented password-field rejection without retaining or
// rendering any server text (which could contain credentials). Wording remains
// live-unverified; unfamiliar messages get generic password requirements advice.
func passwordRejection(data []byte) string {
	var env struct {
		Error struct{ Field, Message string }
	}
	if json.Unmarshal(data, &env) != nil || env.Error.Field != "password" {
		return ""
	}
	message := strings.ToLower(env.Error.Message)
	if strings.Contains(message, "character") {
		return "characters"
	}
	if strings.Contains(message, "strength") || strings.Contains(message, "weak") {
		return "strength"
	}
	return "requirements"
}
