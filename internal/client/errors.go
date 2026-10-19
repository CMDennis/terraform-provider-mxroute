package client

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	Authentication ErrorKind = "authentication"
	Authorization  ErrorKind = "authorization"
	NotFound       ErrorKind = "not_found"
	Conflict       ErrorKind = "conflict"
	Validation     ErrorKind = "validation"
	Business       ErrorKind = "business"
	RateLimited    ErrorKind = "rate_limited"
	Upstream       ErrorKind = "upstream"
	Protocol       ErrorKind = "protocol"
	Transport      ErrorKind = "transport"
	Ambiguous      ErrorKind = "ambiguous"
)

// Error contains only allowlisted metadata. API messages, fields, request URLs,
// bodies and transport error strings are never retained or rendered.
type Error struct {
	Operation       string
	Kind            ErrorKind
	Status          int
	Code            string
	ConfirmedAbsent bool
	Uncertain       bool
	// ReadbackAvailable means a subsequent read succeeded, not that the write
	// was verified. Passwords and delivery side effects cannot be read back.
	ReadbackAvailable bool
	passwordIssue     string // Fixed category only; never server text.
	cause             error
}

func (e *Error) Error() string {
	advice := map[ErrorKind]string{
		Authentication: "check the API credential and DirectAdmin API access settings",
		Authorization:  "check the credential role, key restrictions and account context",
		NotFound:       "verify existence with a complete list in the correct account context",
		Conflict:       "read the existing object before changing configuration",
		Validation:     "check inputs against the documented API constraints",
		Business:       "check account limits and operation prerequisites",
		RateLimited:    "rate limit reached; wait before retrying",
		Upstream:       "upstream service failed; preserve current state",
		Protocol:       "incomplete or invalid API response; preserve current state",
		Transport:      "request transport failed; preserve current state",
		Ambiguous:      "multiple matching objects; preserve current state",
	}[e.Kind]
	switch e.passwordIssue {
	case "characters":
		advice = "mail server rejected password characters; choose characters accepted by the selected server"
	case "strength":
		advice = "mail server rejected password strength; choose a stronger password"
	case "requirements":
		advice = "mail server rejected the password; check its character and strength requirements"
	}
	if e.Uncertain {
		advice += "; write outcome is uncertain: reconcile remote state before retrying; credentials and side effects cannot be verified by readback"
	}
	return fmt.Sprintf("MXroute %s: %s (HTTP %d)", e.Operation, advice, e.Status)
}
func (e *Error) Unwrap() error { return e.cause }

// IsNotFound is intentionally stricter than an HTTP 404 or API NOT_FOUND.
// Only a complete, valid list proving absence can authorize state removal.
func IsNotFound(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == NotFound && e.ConfirmedAbsent
}
func classify(status int, code string) ErrorKind {
	// Status errors take priority over contradictory envelope codes.
	switch status {
	case 401:
		return Authentication
	case 403:
		return Authorization
	case 404:
		return NotFound
	case 409:
		return Conflict
	case 422:
		if code == "BUSINESS_ERROR" {
			return Business
		}
		return Validation
	case 400:
		return Validation
	case 429:
		return RateLimited
	}
	if status >= 500 {
		return Upstream
	}
	switch code {
	case "UNAUTHORIZED":
		return Authentication
	case "FORBIDDEN":
		return Authorization
	case "NOT_FOUND":
		return NotFound
	case "CONFLICT":
		return Conflict
	case "VALIDATION_ERROR":
		return Validation
	case "BUSINESS_ERROR":
		return Business
	case "RATE_LIMITED":
		return RateLimited
	case "SERVER_ERROR", "VERIFICATION_KEY_NOT_FOUND":
		return Upstream
	}
	return Protocol
}
func safeCode(code string) string {
	switch code {
	case "UNAUTHORIZED", "FORBIDDEN", "NOT_FOUND", "CONFLICT", "VALIDATION_ERROR", "BUSINESS_ERROR", "RATE_LIMITED", "SERVER_ERROR", "VERIFICATION_KEY_NOT_FOUND":
		return code
	}
	return ""
}
