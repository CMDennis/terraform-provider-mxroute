// Package schemautil holds Terraform schema conventions shared by future
// resources. API payload types must remain in internal/client.
package schemautil

import (
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// InitialVersion is explicit even for the first schema. Each resource owns its
// future migrations; never register speculative no-op version upgrades.
const InitialVersion int64 = 0

func InitialStateUpgraders() map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{}
}

func PasswordAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"password_wo": schema.StringAttribute{
			Optional: true, Sensitive: true, WriteOnly: true,
			MarkdownDescription: "Write-only password, required on creation and when password_wo_version increases. Read from configuration only; never persisted in plan or state. Omit on updates to preserve the remote credential. Imports do not require a password.",
		},
		"password_wo_version": schema.Int64Attribute{
			Optional:            true,
			MarkdownDescription: "Positive rotation version. Required with password_wo. Increase to rotate; retain the existing version when omitting the password. Imported objects may omit both fields until their first explicit rotation.",
		},
	}
}

// PasswordChange runs during plan modification and again before a write. The
// password is always taken from Config, never Plan/State. It returns intent,
// not a secret or hash. A password change alone cannot cause a Terraform diff.
// min/max are resource-specific documented bounds; max=0 means no known limit.
func PasswordChange(password types.String, version, prior types.Int64, creating, applying bool, min, max int) (bool, diag.Diagnostics) {
	var d diag.Diagnostics
	if !version.IsNull() && !version.IsUnknown() && version.ValueInt64() < 1 {
		d.AddAttributeError(path.Root("password_wo_version"), "Invalid password rotation version", "Use a positive integer.")
	}
	if !password.IsNull() && !password.IsUnknown() {
		n := utf8.RuneCountInString(password.ValueString())
		if n < min || (max > 0 && n > max) {
			d.AddAttributeError(path.Root("password_wo"), "Invalid password length", "The password does not meet this resource's documented length bounds.")
		}
		if version.IsNull() {
			d.AddAttributeError(path.Root("password_wo_version"), "Missing password rotation version", "Set a positive rotation version when supplying a password.")
		}
	}
	if !creating && !prior.IsNull() && !prior.IsUnknown() && !version.IsUnknown() {
		if version.IsNull() || version.ValueInt64() < prior.ValueInt64() {
			d.AddAttributeError(path.Root("password_wo_version"), "Password rotation version decreased", "Retain the current version or increase it for an explicit rotation.")
		}
	}
	if version.IsUnknown() || password.IsUnknown() || (!creating && prior.IsUnknown()) {
		if applying {
			d.AddAttributeError(path.Root("password_wo"), "Unknown password configuration", "Resolve the password and rotation version before applying.")
		}
		return false, d
	}
	rotate := creating || (!version.IsNull() && (prior.IsNull() || version.ValueInt64() != prior.ValueInt64()))
	if rotate && password.IsNull() {
		d.AddAttributeError(path.Root("password_wo"), "Missing password", "Supply a write-only password for creation or an explicit rotation. Imports and updates with an unchanged version do not require one.")
	}
	return rotate && !d.HasError(), d
}
