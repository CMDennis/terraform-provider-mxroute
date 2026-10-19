// mxroute-readcheck requires explicit human authorization before invocation.
// It accepts runtime credentials only and has no write or cleanup capability.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/livecheck"
)

func main() {
	cleanupCheck := len(os.Args) == 3 && (os.Args[1] == "--verify-owned-mailbox-before-delete" || os.Args[1] == "--verify-owned-mailbox-after-delete")
	if !cleanupCheck && (len(os.Args) != 2 || os.Args[1] != "--authorized-read-only") {
		fmt.Fprintln(os.Stderr, "After explicit authorization: mxroute-readcheck --authorized-read-only (credentials via MXROUTE_SERVER, MXROUTE_USERNAME, MXROUTE_API_KEY).")
		os.Exit(2)
	}
	c, err := client.NewReadOnly(os.Getenv("MXROUTE_SERVER"), os.Getenv("MXROUTE_USERNAME"), os.Getenv("MXROUTE_API_KEY"), "phase7-readcheck")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid discovery credentials or account context; details withheld.")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var r livecheck.Report
	if cleanupCheck {
		r = livecheck.VerifyMailboxCleanup(ctx, c, os.Args[2], os.Args[1] == "--verify-owned-mailbox-before-delete")
	} else {
		r = livecheck.Run(ctx, c)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil || !r.Completed || !r.BaselineUnchanged {
		os.Exit(1)
	}
}
