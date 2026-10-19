package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func configFor(t *testing.T, p *mxrouteProvider, m providerModel) tfsdk.Config {
	t.Helper()
	var s provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &s)
	v, d := types.ObjectValueFrom(context.Background(), s.Schema.Type().(types.ObjectType).AttrTypes, m)
	if d.HasError() {
		t.Fatal("cannot encode test provider configuration")
	}
	raw, err := v.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatal("cannot encode test value")
	}
	return tfsdk.Config{Schema: s.Schema, Raw: raw}
}

func TestProviderConfiguration(t *testing.T) {
	testutil.ClearCredentials(t)
	key := testutil.Secret(t)
	t.Setenv("MXROUTE_SERVER", "env.example.com")
	t.Setenv("MXROUTE_USERNAME", "envowner|envuser")
	t.Setenv("MXROUTE_API_KEY", key)
	null := types.StringNull()
	for _, tc := range []struct {
		name             string
		m                providerModel
		invalid, unknown bool
	}{
		{"environment", providerModel{null, null, null}, false, false},
		{"configuration", providerModel{types.StringValue("config.example.com"), types.StringValue("configowner"), types.StringValue(key)}, false, false},
		{"empty overrides environment", providerModel{null, null, types.StringValue("")}, true, false},
		{"whitespace overrides environment", providerModel{null, types.StringValue(" "), null}, true, false},
		{"unknown does not fall back", providerModel{null, types.StringUnknown(), null}, false, true},
		{"invalid server", providerModel{types.StringValue("https://example.com"), null, null}, true, false},
		{"invalid authentication", providerModel{null, types.StringValue("owner|user|extra"), null}, true, false},
		{"invalid header", providerModel{null, null, types.StringValue(key + "\n")}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := &mxrouteProvider{version: "test", newClient: func(server, username, apiKey, version string) (*client.Client, error) {
				calls++
				if tc.name == "environment" && (server != "env.example.com" || username != "envowner|envuser" || apiKey != key) {
					t.Error("environment fallback failed")
				}
				if tc.name == "configuration" && (server != "config.example.com" || username != "configowner" || apiKey != key) {
					t.Error("explicit configuration did not take precedence")
				}
				return client.NewMock(server, username, apiKey, version, "http://127.0.0.1:1")
			}}
			cfg := configFor(t, p, tc.m)
			var validation provider.ValidateConfigResponse
			p.ValidateConfig(context.Background(), provider.ValidateConfigRequest{Config: cfg}, &validation)
			if validation.Diagnostics.HasError() != tc.invalid {
				t.Fatal("incorrect validation result")
			}
			var response provider.ConfigureResponse
			p.Configure(context.Background(), provider.ConfigureRequest{Config: cfg}, &response)
			if response.Diagnostics.HasError() != (tc.invalid || tc.unknown) {
				t.Fatal("incorrect configuration result")
			}
			if tc.invalid || tc.unknown {
				if calls != 0 || response.ResourceData != nil {
					t.Fatal("invalid configuration initialized a client")
				}
			} else {
				if calls != 1 || response.ResourceData != response.DataSourceData {
					t.Fatal("client not shared with resource and data source constructors")
				}
			}
			for _, d := range response.Diagnostics {
				if strings.Contains(d.Detail(), key) {
					t.Fatal("diagnostic leaked credential")
				}
			}
		})
	}
}

func TestProviderMissingAndMetadata(t *testing.T) {
	testutil.ClearCredentials(t)
	p := New("test")().(*mxrouteProvider)
	cfg := configFor(t, p, providerModel{types.StringNull(), types.StringNull(), types.StringNull()})
	var response provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{Config: cfg}, &response)
	if len(response.Diagnostics) != 3 || !response.Diagnostics.HasError() {
		t.Fatal("missing provider values were not individually diagnosed")
	}
	var meta provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &meta)
	if meta.TypeName != "mxroute" || meta.Version != "test" {
		t.Fatal("incorrect provider metadata")
	}
	var schema provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &schema)
	if !schema.Schema.Attributes["api_key"].IsSensitive() || len(schema.Schema.Attributes) != 3 {
		t.Fatal("provider schema changed unexpectedly")
	}
	if len(p.Resources(context.Background())) != 10 || len(p.DataSources(context.Background())) != 20 {
		t.Fatal("unexpected production resource/data source count")
	}
}
