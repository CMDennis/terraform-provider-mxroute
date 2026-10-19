package provider

import (
	"context"
	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSpamValidators(t *testing.T) {
	for _, v := range []types.Int64{types.Int64Null(), types.Int64Unknown(), types.Int64Value(0), types.Int64Value(1), types.Int64Value(50), types.Int64Value(51)} {
		var resp validator.Int64Response
		scoreValidator{}.ValidateInt64(context.Background(), validator.Int64Request{Path: path.Root("high_score"), ConfigValue: v}, &resp)
		invalid := !v.IsNull() && !v.IsUnknown() && (v.ValueInt64() < 1 || v.ValueInt64() > 50)
		if resp.Diagnostics.HasError() != invalid {
			t.Fatal("invalid score bounds or deferred value handling")
		}
	}
	for _, tc := range []struct {
		value types.String
		valid bool
	}{
		{types.StringNull(), true}, {types.StringUnknown(), true}, {types.StringValue("*@Example.NET"), true}, {types.StringValue("a+b=c@example.net"), true},
		{types.StringValue(strings.Repeat("a", 254)), true}, {types.StringValue(strings.Repeat("a", 255)), false}, {types.StringValue(""), false}, {types.StringValue("x/y@example.net"), false}, {types.StringValue("a b@example.net"), false},
	} {
		var resp validator.StringResponse
		entryValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("entry"), ConfigValue: tc.value}, &resp)
		if resp.Diagnostics.HasError() == tc.valid {
			t.Fatal("invalid entry bounds, characters or deferred value handling")
		}
	}
}

func TestSpamCreateRetainsIdentityAfterFailedReadback(t *testing.T) {
	for _, kind := range []string{"settings", "whitelist_entry", "blacklist_entry"} {
		t.Run(kind, func(t *testing.T) {
			changed := false
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method != "GET" {
					changed = true
					if r.Method == "POST" {
						w.WriteHeader(201)
					}
					return
				}
				if changed {
					_, _ = w.Write([]byte(`{"success":true}`))
					return
				}
				if kind == "settings" {
					_, _ = w.Write([]byte(`{"success":true,"data":{"high_score":15}}`))
				} else {
					_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
				}
			}))
			defer s.Close()
			c, err := client.NewMock("mail.example.com", "owner", testutil.Secret(t), "test", s.URL)
			if err != nil {
				t.Fatal(err)
			}
			r := &spamResource{kind: kind, c: c}
			ctx := context.Background()
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			if kind == "settings" {
				if ds := state.Set(ctx, &spamScoreModel{ID: types.StringUnknown(), Domain: types.StringValue("example.com"), Score: types.Int64Value(20)}); ds.HasError() {
					t.Fatal(ds)
				}
			} else {
				if ds := state.Set(ctx, &spamModel{ID: types.StringUnknown(), Domain: types.StringValue("example.com"), Entry: types.StringValue("*@Example.NET")}); ds.HasError() {
					t.Fatal(ds)
				}
			}
			resp := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: schema.Schema, Raw: state.Raw}}, &resp)
			if !resp.Diagnostics.HasError() || !changed {
				t.Fatal("unconfirmed write must report a failure")
			}
			var id types.String
			if ds := resp.State.GetAttribute(ctx, path.Root("id"), &id); ds.HasError() {
				t.Fatal(ds)
			}
			expected := "*@example.net"
			if kind == "settings" {
				expected = "spam-settings"
			}
			if id.ValueString() != expected {
				t.Fatal("unconfirmed create lost its recovery identity")
			}
		})
	}
}
