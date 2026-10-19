package schemautil

import (
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPasswordIntent(t *testing.T) {
	pw := types.StringValue(testutil.Secret(t))
	null := types.Int64Null()
	one, two := types.Int64Value(1), types.Int64Value(2)
	for _, tc := range []struct {
		name                           string
		pw                             types.String
		version, prior                 types.Int64
		create, apply, rotate, invalid bool
	}{
		{"create", pw, one, null, true, true, true, false},
		{"create missing", types.StringNull(), null, null, true, false, false, true},
		{"create unknown", types.StringUnknown(), one, null, true, false, false, false},
		{"apply unknown", types.StringUnknown(), one, null, true, true, false, true},
		{"import unchanged", types.StringNull(), null, null, false, true, false, false},
		{"omit on update", types.StringNull(), one, one, false, true, false, false},
		{"unchanged supplied", pw, one, one, false, true, false, false},
		{"rotate", pw, two, one, false, true, true, false},
		{"first imported rotation", pw, one, null, false, true, true, false},
		{"missing rotation password", types.StringNull(), two, one, false, false, false, true},
		{"no version", pw, null, null, false, false, false, true},
		{"decreased", pw, one, two, false, false, false, true},
		{"removed version", types.StringNull(), null, one, false, false, false, true},
		{"zero version", pw, types.Int64Value(0), null, true, false, false, true},
		{"empty password", types.StringValue(""), one, null, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rotate, d := PasswordChange(tc.pw, tc.version, tc.prior, tc.create, tc.apply, 8, 72)
			if rotate != tc.rotate || d.HasError() != tc.invalid {
				t.Fatal("incorrect rotation intent or validation")
			}
			for _, diagnostic := range d {
				if strings.Contains(diagnostic.Detail(), pw.ValueString()) {
					t.Fatal("password leaked in diagnostic")
				}
			}
		})
	}
	attr := PasswordAttributes()["password_wo"]
	if !attr.IsWriteOnly() || !attr.IsSensitive() || !attr.IsOptional() || attr.IsComputed() {
		t.Fatal("unsafe password schema")
	}
}
