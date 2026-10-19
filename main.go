package main

import (
	"context"
	"flag"
	"log"

	"github.com/CMDennis/terraform-provider-mxroute/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Set by the release build. Development binaries deliberately identify as dev.
var version = "dev"

func main() {
	debug := flag.Bool("debug", false, "enable debugger support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/cmdennis/mxroute", ProtocolVersion: 6, Debug: *debug,
	}); err != nil {
		log.Fatal(err)
	}
}
