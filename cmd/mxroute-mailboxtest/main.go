// Opt-in test of one new mailbox; never a Terraform acceptance harness.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/livecheck"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--authorized-new-mailbox-retain" {
		fmt.Fprintln(os.Stderr, "Requires explicit authorization to create/update one new mailbox and retain it. Supply credentials only through runtime MXROUTE_* variables.")
		os.Exit(2)
	}
	server, user, key := os.Getenv("MXROUTE_SERVER"), os.Getenv("MXROUTE_USERNAME"), os.Getenv("MXROUTE_API_KEY")
	reader, err := client.NewReadOnly(server, user, key, "phase7-mailboxtest")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid runtime credentials or account context; details withheld.")
		os.Exit(2)
	}
	dir, err := os.MkdirTemp("/tmp", "mxroute-mailboxtest-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot create protected private test directory.")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "Private test record:", filepath.Join(dir, "manifest.json"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	r := livecheck.RunMailbox(ctx, reader, func(domain, local string) (*client.Client, error) {
		return client.NewMailboxTest(server, user, key, "phase7-mailboxtest", domain, local)
	}, dir, func(status string) { fmt.Fprintln(os.Stderr, status) })
	if json.NewEncoder(os.Stdout).Encode(r) != nil || !r.Completed || !r.BaselineUnchanged {
		os.Exit(1)
	}
}
