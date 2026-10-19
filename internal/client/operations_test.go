package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

type schema map[string]any
type baselineOperation struct {
	Key          string `json:"key"`
	ClientMethod string `json:"client_method"`
	Responses    map[string]struct {
		Content map[string]struct {
			Schema schema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}
type baseline struct {
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
	Operations []baselineOperation `json:"operations"`
}

func loadBaseline(t *testing.T) baseline {
	t.Helper()
	data, err := os.ReadFile("../../api/operations.json")
	if err != nil {
		t.Fatal("baseline unavailable")
	}
	var b baseline
	if json.Unmarshal(data, &b) != nil {
		t.Fatal("invalid baseline")
	}
	return b
}
func sample(b baseline, s schema) any {
	if ref, ok := s["$ref"].(string); ok {
		return sample(b, b.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")])
	}
	if s["nullable"] == true {
		return nil
	}
	switch s["type"] {
	case "object":
		v := map[string]any{}
		for k, prop := range s["properties"].(map[string]any) {
			v[k] = sample(b, schema(prop.(map[string]any)))
		}
		if _, ok := v["success"]; ok {
			v["success"] = true
		}
		return v
	case "array":
		return []any{sample(b, schema(s["items"].(map[string]any)))}
	case "string":
		if enums, ok := s["enum"].([]any); ok {
			return enums[0]
		}
		return "example.com"
	case "boolean":
		return false
	case "integer", "number":
		return float64(0)
	}
	panic("unhandled baseline schema")
}
func ptr[T any](value T) *T { return &value }
func newMock(t *testing.T, handler http.HandlerFunc, username string) *Client {
	t.Helper()
	testutil.ClearCredentials(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := NewMock("mail.example.com", username, testutil.Secret(t)+":"+testutil.Secret(t), "phase2-test", server.URL)
	if err != nil {
		t.Fatal("cannot create mock client")
	}
	c.retryBase = time.Millisecond
	return c
}

// Inputs and expected JSON are authored independently of the client methods.
// Each case exercises explicit zero/false values and a generated password.
func operationInput(name, password string) (any, map[string]any) {
	switch name {
	case "CreateDomain":
		return CreateDomainRequest{Domain: "example.com"}, map[string]any{"domain": "example.com"}
	case "CreateDomainPointer":
		return CreateDomainPointerRequest{Pointer: "alias.example.net", Alias: ptr(false)}, map[string]any{"pointer": "alias.example.net", "alias": false}
	case "CreateEmailAccount":
		return CreateEmailAccountRequest{Username: "team", Password: password, Quota: ptr(int64(0)), Limit: ptr(int64(0))}, map[string]any{"username": "team", "password": password, "quota": float64(0), "limit": float64(0)}
	case "UpdateEmailAccount":
		return UpdateEmailAccountRequest{Password: &password, Quota: ptr(int64(0)), Limit: ptr(int64(0))}, map[string]any{"password": password, "quota": float64(0), "limit": float64(0)}
	case "CreateForwarder":
		return CreateForwarderRequest{Alias: "support", Destinations: []string{"help@example.net"}}, map[string]any{"alias": "support", "destinations": []any{"help@example.net"}}
	case "SetCatchAll":
		return SetCatchAllRequest{Type: "address", Address: ptr("help@example.net")}, map[string]any{"type": "address", "address": "help@example.net"}
	case "CreateResellerUser":
		return CreateResellerUserRequest{Username: "new_user", Email: "owner@example.net", Password: password, Package: "starter"}, map[string]any{"username": "new_user", "email": "owner@example.net", "password": password, "package": "starter"}
	case "UpdateResellerUser":
		return UpdateResellerUserRequest{Quota: ptr("0"), Password: &password}, map[string]any{"quota": "0", "password": password}
	case "CreateResellerPackage":
		return CreateResellerPackageRequest{Name: "starter", PackageLimits: PackageLimits{Quota: ptr("unlimited"), Domains: ptr("0"), EmailAccounts: ptr("0"), EmailForwarders: ptr("0"), DomainPointers: ptr("0")}}, map[string]any{"name": "starter", "quota": "unlimited", "domains": "0", "email_accounts": "0", "email_forwarders": "0", "domain_pointers": "0"}
	case "UpdateResellerPackage":
		return PackageLimits{Quota: ptr("unlimited"), Domains: ptr("0"), EmailAccounts: ptr("0"), EmailForwarders: ptr("0"), DomainPointers: ptr("0")}, map[string]any{"quota": "unlimited", "domains": "0", "email_accounts": "0", "email_forwarders": "0", "domain_pointers": "0"}
	case "SetMailHosting":
		return false, map[string]any{"enabled": false}
	case "UpdateSpamSettings":
		return int64(5), map[string]any{"high_score": float64(5)}
	case "AddSpamWhitelistEntry", "AddSpamBlacklistEntry":
		return "*@trusted.example", map[string]any{"entry": "*@trusted.example"}
	case "SetResellerUserPackage":
		return "starter", map[string]any{"package": "starter"}
	}
	return nil, nil
}
func operationArgs(op baselineOperation, password string) ([]reflect.Value, string, map[string]any) {
	_, path, _ := strings.Cut(op.Key, " ")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	args := []reflect.Value{reflect.ValueOf(context.Background())}
	for i, s := range segments {
		if strings.HasPrefix(s, "{") {
			value := map[string]string{"{domain}": "example.com", "{pointer}": "alias.example.net", "{user}": "Team+ops@example.com", "{alias}": "support+ops@example.com", "{entry}": "*@trusted.example", "{username}": "ordinary", "{name}": "starter"}[s]
			segments[i] = value
			args = append(args, reflect.ValueOf(value))
		}
	}
	input, expected := operationInput(op.ClientMethod, password)
	if input != nil {
		args = append(args, reflect.ValueOf(input))
	}
	return args, identity.Path(segments...), expected
}
func testOperation(t *testing.T, name string) {
	t.Helper()
	b := loadBaseline(t)
	var op baselineOperation
	for _, candidate := range b.Operations {
		if candidate.ClientMethod == name {
			op = candidate
			break
		}
	}
	if op.Key == "" {
		t.Fatal("operation not in pinned baseline")
	}
	method, _, _ := strings.Cut(op.Key, " ")
	args, path, expected := operationArgs(op, testutil.Secret(t))
	status := 200
	if method == "POST" && !strings.Contains(name, "suspend") && !strings.Contains(name, "Suspend") {
		status = 201
	}
	if method == "DELETE" {
		status = 204
	}
	// Use documented success statuses, including action POST 200 responses.
	for value := range op.Responses {
		if strings.HasPrefix(value, "2") {
			_, _ = fmt.Sscan(value, &status)
		}
	}
	var response any
	if s := op.Responses[fmt.Sprint(status)].Content["application/json"].Schema; s != nil {
		response = sample(b, s)
	}
	// Blacklist has no published schema; the whitelist-shaped response remains
	// an explicitly synthetic assumption, tested fail-closed on other shapes.
	if name == "GetSpamBlacklist" {
		response = map[string]any{"success": true, "data": []any{"*@trusted.example"}}
	}
	called := 0
	var c *Client
	c = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.Method != method || r.URL.EscapedPath() != path {
			t.Error("wrong HTTP method or escaped path")
		}
		if r.Header.Get("X-Server") != c.server || r.Header.Get("X-Username") != c.username || r.Header.Get("X-API-Key") != c.apiKey || r.UserAgent() != "terraform-provider-mxroute/phase2-test" {
			t.Error("configured headers changed")
		}
		data, _ := io.ReadAll(r.Body)
		if expected == nil {
			if len(data) != 0 {
				t.Error("unexpected request body")
			}
		} else {
			var actual map[string]any
			if json.Unmarshal(data, &actual) != nil || !reflect.DeepEqual(actual, expected) {
				t.Error("request JSON does not match the operation contract")
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing request content type")
			}
		}
		if response != nil {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		if response != nil {
			_ = json.NewEncoder(w).Encode(response)
		}
	}, "owner")
	fn := reflect.ValueOf(c).MethodByName(name)
	if !fn.IsValid() {
		t.Fatal("missing typed client method")
	}
	result := fn.Call(args)
	if !result[len(result)-1].IsNil() {
		t.Fatal("documented success response was rejected")
	}
	if called != 1 {
		t.Fatal("success request repeated")
	}
	if len(result) == 2 {
		data, err := json.Marshal(result[0].Interface())
		if err != nil {
			t.Fatal("cannot inspect decoded model")
		}
		var actual any
		_ = json.Unmarshal(data, &actual)
		want := response
		if env, ok := response.(map[string]any); ok {
			if d, has := env["data"]; has {
				want = d
			}
		}
		// Omitted optional descriptions marshal as null in typed responses.
		var clean func(any)
		clean = func(v any) {
			switch v := v.(type) {
			case map[string]any:
				for k, x := range v {
					if k == "description" && x == nil {
						delete(v, k)
					} else {
						clean(x)
					}
				}
			case []any:
				for _, x := range v {
					clean(x)
				}
			}
		}
		clean(actual)
		clean(want)
		if !reflect.DeepEqual(actual, want) {
			t.Error("decoded response differs from baseline fields, zero values or nulls")
		}
	}
}
func TestClient_DeleteDomain(t *testing.T)             { testOperation(t, "DeleteDomain") }
func TestClient_DeleteEmailAccount(t *testing.T)       { testOperation(t, "DeleteEmailAccount") }
func TestClient_DeleteForwarder(t *testing.T)          { testOperation(t, "DeleteForwarder") }
func TestClient_DeleteDomainPointer(t *testing.T)      { testOperation(t, "DeleteDomainPointer") }
func TestClient_DeleteSpamBlacklistEntry(t *testing.T) { testOperation(t, "DeleteSpamBlacklistEntry") }
func TestClient_DeleteSpamWhitelistEntry(t *testing.T) { testOperation(t, "DeleteSpamWhitelistEntry") }
func TestClient_DeleteResellerPackage(t *testing.T)    { testOperation(t, "DeleteResellerPackage") }
func TestClient_DeleteResellerUser(t *testing.T)       { testOperation(t, "DeleteResellerUser") }
func TestClient_ListDomains(t *testing.T)              { testOperation(t, "ListDomains") }
func TestClient_GetDomain(t *testing.T)                { testOperation(t, "GetDomain") }
func TestClient_GetCatchAll(t *testing.T)              { testOperation(t, "GetCatchAll") }
func TestClient_GetDNSRecords(t *testing.T)            { testOperation(t, "GetDNSRecords") }
func TestClient_ListEmailAccounts(t *testing.T)        { testOperation(t, "ListEmailAccounts") }
func TestClient_GetEmailAccount(t *testing.T)          { testOperation(t, "GetEmailAccount") }
func TestClient_ListForwarders(t *testing.T)           { testOperation(t, "ListForwarders") }
func TestClient_ListDomainPointers(t *testing.T)       { testOperation(t, "ListDomainPointers") }
func TestClient_GetSpamBlacklist(t *testing.T)         { testOperation(t, "GetSpamBlacklist") }
func TestClient_GetSpamSettings(t *testing.T)          { testOperation(t, "GetSpamSettings") }
func TestClient_GetSpamWhitelist(t *testing.T)         { testOperation(t, "GetSpamWhitelist") }
func TestClient_GetQuota(t *testing.T)                 { testOperation(t, "GetQuota") }
func TestClient_GetEmailUsage(t *testing.T)            { testOperation(t, "GetEmailUsage") }
func TestClient_ListResellerPackages(t *testing.T)     { testOperation(t, "ListResellerPackages") }
func TestClient_GetResellerPackage(t *testing.T)       { testOperation(t, "GetResellerPackage") }
func TestClient_ListResellerUsers(t *testing.T)        { testOperation(t, "ListResellerUsers") }
func TestClient_GetResellerUser(t *testing.T)          { testOperation(t, "GetResellerUser") }
func TestClient_GetVerificationKey(t *testing.T)       { testOperation(t, "GetVerificationKey") }
func TestClient_SetCatchAll(t *testing.T)              { testOperation(t, "SetCatchAll") }
func TestClient_UpdateEmailAccount(t *testing.T)       { testOperation(t, "UpdateEmailAccount") }
func TestClient_SetMailHosting(t *testing.T)           { testOperation(t, "SetMailHosting") }
func TestClient_UpdateSpamSettings(t *testing.T)       { testOperation(t, "UpdateSpamSettings") }
func TestClient_UpdateResellerPackage(t *testing.T)    { testOperation(t, "UpdateResellerPackage") }
func TestClient_UpdateResellerUser(t *testing.T)       { testOperation(t, "UpdateResellerUser") }
func TestClient_SetResellerUserPackage(t *testing.T)   { testOperation(t, "SetResellerUserPackage") }
func TestClient_CreateDomain(t *testing.T)             { testOperation(t, "CreateDomain") }
func TestClient_CreateEmailAccount(t *testing.T)       { testOperation(t, "CreateEmailAccount") }
func TestClient_CreateForwarder(t *testing.T)          { testOperation(t, "CreateForwarder") }
func TestClient_CreateDomainPointer(t *testing.T)      { testOperation(t, "CreateDomainPointer") }
func TestClient_AddSpamBlacklistEntry(t *testing.T)    { testOperation(t, "AddSpamBlacklistEntry") }
func TestClient_AddSpamWhitelistEntry(t *testing.T)    { testOperation(t, "AddSpamWhitelistEntry") }
func TestClient_CreateResellerPackage(t *testing.T)    { testOperation(t, "CreateResellerPackage") }
func TestClient_CreateResellerUser(t *testing.T)       { testOperation(t, "CreateResellerUser") }
func TestClient_SuspendResellerUser(t *testing.T)      { testOperation(t, "SuspendResellerUser") }
func TestClient_UnsuspendResellerUser(t *testing.T)    { testOperation(t, "UnsuspendResellerUser") }
