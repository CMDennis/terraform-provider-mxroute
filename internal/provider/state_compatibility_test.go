package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// No released state migrations exist yet. Freeze all resource v0 types and
// persistence flags so future schema edits require an explicit compatibility
// review and real state-upgrade fixtures instead of speculative no-op upgrades.
func TestResourceSchemaCompatibilityBaseline(t *testing.T) {
	testutil.ClearCredentials(t)
	server := providerserver.NewProtocol6(New("test")())()
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal("cannot obtain resource schemas")
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatal("resource schema diagnostics contain errors")
		}
	}
	for _, s := range resp.ResourceSchemas {
		if s.Version != 0 {
			t.Fatal("new schema version requires state upgrade tests and a revised compatibility baseline")
		}
	}
	data, err := json.Marshal(resp.ResourceSchemas)
	if err != nil {
		t.Fatal("cannot serialize schema compatibility baseline")
	}
	var tree any
	if json.Unmarshal(data, &tree) != nil {
		t.Fatal("cannot decode schema baseline")
	}
	var removeProse func(any)
	removeProse = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			delete(x, "Description")
			delete(x, "DescriptionKind")
			for _, child := range x {
				removeProse(child)
			}
		case []any:
			for _, child := range x {
				removeProse(child)
			}
		}
	}
	removeProse(tree)
	data, _ = json.MarshalIndent(tree, "", "  ")
	data = append(data, '\n')
	const fixture = "../../testdata/compatibility/resource-schemas-v0.json"
	if os.Getenv("UPDATE_MXROUTE_SCHEMA_FIXTURE") == "1" {
		if os.WriteFile(fixture, data, 0644) != nil {
			t.Fatal("cannot write explicitly requested synthetic schema baseline")
		}
	}
	expected, err := os.ReadFile(fixture)
	if err != nil || !bytes.Equal(data, expected) {
		t.Fatal("resource schema differs from the v0 compatibility baseline; review state migration and persistence implications")
	}
}
