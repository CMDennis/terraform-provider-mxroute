package compatibility

import (
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"testing"
)

func TestSelectedAPIsCompile(t *testing.T) {
	attr := schema.StringAttribute{Optional: true, WriteOnly: true}
	opts := providerserver.ServeOpts{ProtocolVersion: 6}
	testCase := resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){}}
	if !attr.WriteOnly || opts.ProtocolVersion != 6 || testCase.ProtoV6ProviderFactories == nil {
		t.Fatal("compatibility check failed")
	}
}
