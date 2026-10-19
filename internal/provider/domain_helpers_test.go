package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDomainAbsenceRequiresCompleteList(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		absent     bool
	}{
		{"absent", `{"success":true,"data":[]}`, 200, true},
		{"still present", `{"success":true,"data":["example.com"]}`, 200, false},
		{"ambiguous", `{"success":true,"data":["example.com","EXAMPLE.COM"]}`, 200, false},
		{"incomplete", `{"success":true}`, 200, false},
		{"unavailable", `{"success":false,"error":{"code":"FORBIDDEN"}}`, 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/domains" {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				w.WriteHeader(404)
			}))
			defer s.Close()
			c, err := client.NewMock("mail.example.com", "owner", testutil.Secret(t), "test", s.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = readDomain(context.Background(), c, "example.com")
			if err == nil || client.IsNotFound(err) != tc.absent {
				t.Fatal("invalid absence classification")
			}
		})
	}
}
func TestDomainValidators(t *testing.T) {
	for _, value := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("example.com"), types.StringValue("EXAMPLE.COM"), types.StringValue("example.com."), types.StringValue("bad/name")} {
		var resp validator.StringResponse
		domainValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("domain"), ConfigValue: value}, &resp)
		valid := value.IsNull() || value.IsUnknown() || value.ValueString() == "example.com"
		if resp.Diagnostics.HasError() == valid {
			t.Fatal("incorrect canonical domain validation")
		}
	}
	for _, value := range []int64{-1, 0, 1800, 1801} {
		var resp validator.Int64Response
		waitValidator{}.ValidateInt64(context.Background(), validator.Int64Request{Path: path.Root("wait"), ConfigValue: types.Int64Value(value)}, &resp)
		if resp.Diagnostics.HasError() != (value < 0 || value > 1800) {
			t.Fatal("incorrect bounded wait validation")
		}
	}
	for _, value := range []string{"alias", "redirect", "other"} {
		var resp validator.StringResponse
		modeValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("type"), ConfigValue: types.StringValue(value)}, &resp)
		if resp.Diagnostics.HasError() != (value == "other") {
			t.Fatal("incorrect pointer mode validation")
		}
	}
}
func TestReadinessWaitStopsOnUnrelatedErrors(t *testing.T) {
	sentinel := errors.New("unrelated failure")
	calls := 0
	err := poll(context.Background(), 1800, func(context.Context) (bool, error) { calls++; return false, sentinel })
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatal("unrelated error retried")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := poll(ctx, 1800, func(context.Context) (bool, error) { return false, nil }); err == nil {
		t.Fatal("cancellation did not stop wait")
	}
	calls = 0
	if err := poll(context.Background(), 0, func(context.Context) (bool, error) { calls++; return false, nil }); err == nil || calls != 1 {
		t.Fatal("zero wait did not check exactly once")
	}
}

func TestPointerDeletionNeverAssumesParentCascade(t *testing.T) {
	for _, status := range []int{200, 404, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/domains" {
					_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
					return
				}
				if r.URL.Path == "/domains/example.com/pointers" {
					w.WriteHeader(status)
					if status == 200 {
						_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
					}
					return
				}
				w.WriteHeader(404)
			}))
			defer s.Close()
			c, err := client.NewMock("mail.example.com", "owner", testutil.Secret(t), "test", s.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = readPointer(context.Background(), c, "example.com", "alias.example.net")
			if client.IsNotFound(err) != (status == 200) {
				t.Fatal("parent absence was mistaken for confirmed pointer deletion")
			}
		})
	}
}
