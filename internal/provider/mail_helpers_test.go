package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMailValidators(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		value string
		valid bool
	}{{"Support", true}, {"support+tag", true}, {"", false}, {"support@example.com", false}, {"a/b", false}, {"a b", false}, {"..", false}} {
		var resp validator.StringResponse
		localValidator{}.ValidateString(ctx, validator.StringRequest{Path: path.Root("username"), ConfigValue: types.StringValue(tc.value)}, &resp)
		if resp.Diagnostics.HasError() == tc.valid {
			t.Fatal("incorrect local-part validation")
		}
	}
	for _, v := range []types.Int64{types.Int64Null(), types.Int64Unknown(), types.Int64Value(0), types.Int64Value(9600), types.Int64Value(9601)} {
		var resp validator.Int64Response
		sendLimitValidator{}.ValidateInt64(ctx, validator.Int64Request{Path: path.Root("limit"), ConfigValue: v}, &resp)
		if resp.Diagnostics.HasError() != (!v.IsUnknown() && v.ValueInt64() > 9600) {
			t.Fatal("incorrect send-limit ceiling")
		}
	}
	for _, tc := range []struct {
		values []string
		valid  bool
	}{
		{[]string{"one@example.net", "two@example.org"}, true}, {[]string{":fail:"}, true}, {[]string{":blackhole:"}, true},
		{[]string{":fail:", "one@example.net"}, false}, {[]string{":blackhole:", ":fail:"}, false}, {[]string{}, false},
		{[]string{"Name <one@example.net>"}, false}, {[]string{"not-an-address"}, false},
	} {
		v, ds := types.SetValueFrom(ctx, types.StringType, tc.values)
		if ds.HasError() {
			t.Fatal("test encoding failed")
		}
		var resp validator.SetResponse
		destinationsValidator{}.ValidateSet(ctx, validator.SetRequest{Path: path.Root("destinations"), ConfigValue: v}, &resp)
		if resp.Diagnostics.HasError() == tc.valid {
			t.Fatal("incorrect destination validation")
		}
	}
	for _, tc := range []struct {
		mode  string
		addr  types.String
		valid bool
	}{
		{"address", types.StringValue("catch@example.net"), true}, {"address", types.StringNull(), false}, {"address", types.StringUnknown(), true},
		{"fail", types.StringNull(), true}, {"blackhole", types.StringNull(), true}, {"fail", types.StringValue("catch@example.net"), false},
		{"blackhole", types.StringValue(""), false}, {"other", types.StringNull(), false},
	} {
		var d diag.Diagnostics
		validateCatchAll(catchAllModel{Type: types.StringValue(tc.mode), Address: tc.addr}, &d)
		if d.HasError() == tc.valid {
			t.Fatal("incorrect conditional catch-all address validation")
		}
	}
	// Length boundaries are tested using generated secrets, never saved literals.
	secret := testutil.Secret(t)
	for _, n := range []int{7, 8, 72, 73} {
		password := strings.Repeat(secret, 3)[:n]
		_, d := schemautil.PasswordChange(types.StringValue(password), types.Int64Value(1), types.Int64Null(), true, true, 8, 72)
		if d.HasError() != (n < 8 || n > 72) {
			t.Fatal("incorrect mailbox password length boundary")
		}
	}
}
func TestMailAbsenceRequiresCompleteList(t *testing.T) {
	for _, kind := range []string{"email-accounts", "forwarders"} {
		for _, tc := range []struct {
			name   string
			status int
			data   any
			absent bool
		}{
			{"empty", 200, []any{}, true}, {"missing envelope", 200, nil, false}, {"parent unavailable", 404, nil, false}, {"forbidden", 403, nil, false},
		} {
			t.Run(kind+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					if tc.data != nil {
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": tc.data})
					}
				}))
				defer server.Close()
				c, err := client.NewMock("mail.example.com", "owner", testutil.Secret(t), "test", server.URL)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "email-accounts" {
					_, err = readMailbox(context.Background(), c, "example.com", "support")
				} else {
					_, err = readForwarder(context.Background(), c, "example.com", "support")
				}
				if err == nil || client.IsNotFound(err) != tc.absent {
					t.Fatal("unconfirmed absence could erase state")
				}
			})
		}
	}
}
