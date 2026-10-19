// Package testutil contains credential-free local test helpers only.
package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// Secret creates disposable inputs in memory; never store passwords in fixtures.
func Secret(t testing.TB) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal("unable to generate disposable test input")
	}
	return hex.EncodeToString(b)
}

func ClearCredentials(t *testing.T) {
	t.Helper()
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(name, "TF_LOG") || strings.HasPrefix(name, "TF_VAR_") || strings.HasPrefix(name, "TF_CLI_ARGS") {
			t.Setenv(name, "")
		}
	}
	for _, name := range []string{"MXROUTE_SERVER", "MXROUTE_USERNAME", "MXROUTE_API_KEY", "TF_REATTACH_PROVIDERS", "TF_DATA_DIR", "TF_WORKSPACE", "TF_ACC_LOG_PATH"} {
		t.Setenv(name, "")
	}
}
