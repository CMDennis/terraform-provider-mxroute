package provider

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

// Exercise the production binary, installed from its real release ZIP. A
// CONNECT proxy routes only the API hostname to a fixed loopback TLS fixture;
// it never resolves or forwards a request to an external destination. The
// production binary has no test endpoint switch or test-only provider code.
func TestOfflineReleasedProviderInstallation(t *testing.T) {
	archive := os.Getenv("MXROUTE_RELEASE_ARCHIVE")
	if archive == "" {
		t.Skip("set MXROUTE_RELEASE_ARCHIVE to a native release ZIP; make release-smoke requires it")
	}
	pattern := `^terraform-provider-mxroute_([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)_` + runtime.GOOS + `_` + runtime.GOARCH + `\.zip$`
	match := regexp.MustCompile(pattern).FindStringSubmatch(filepath.Base(archive))
	if match == nil {
		t.Fatal("release ZIP must be named for the native platform and a semantic version")
	}
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal("cannot read release ZIP")
	}
	testutil.ClearCredentials(t)
	dir := t.TempDir()
	key, password := testutil.Secret(t), testutil.Secret(t)
	checks := secretChecks{secrets: []string{key, password}, dir: dir}
	s := &mailService{key: key, password: password, accounts: map[string]map[string]any{}}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("cannot generate temporary TLS identity")
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Offline release fixture"},
		DNSNames: []string{"api.mxroute.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &certKey.PublicKey, certKey)
	if err != nil {
		t.Fatal("cannot create temporary TLS certificate")
	}
	mock := httptest.NewUnstartedServer(s.serve(t))
	mock.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: certKey}}}
	mock.StartTLS()
	t.Cleanup(mock.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "api.mxroute.com:443" {
			t.Error("offline proxy blocked an unexpected destination")
			http.Error(w, "offline destination blocked", http.StatusForbidden)
			return
		}
		upstream, dialErr := net.DialTimeout("tcp", mock.Listener.Addr().String(), 5*time.Second)
		if dialErr != nil {
			http.Error(w, "local fixture unavailable", http.StatusBadGateway)
			return
		}
		downstream, _, hijackErr := w.(http.Hijacker).Hijack()
		if hijackErr != nil {
			_ = upstream.Close()
			return
		}
		_, _ = io.WriteString(downstream, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, downstream); _ = upstream.Close() }()
		go func() { _, _ = io.Copy(downstream, upstream); _ = downstream.Close() }()
	}))
	t.Cleanup(proxy.Close)
	ca := filepath.Join(t.TempDir(), "root.crt")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal("cannot write temporary public CA")
	}
	mirror := t.TempDir()
	packageDir := filepath.Join(mirror, "registry.terraform.io", "cmdennis", "mxroute")
	if err := os.MkdirAll(packageDir, 0700); err != nil {
		t.Fatal("cannot create isolated filesystem mirror")
	}
	if err := os.WriteFile(filepath.Join(packageDir, filepath.Base(archive)), archiveBytes, 0600); err != nil {
		t.Fatal("cannot populate isolated mirror")
	}
	cliConfig := filepath.Join(t.TempDir(), "terraform.rc")
	config := fmt.Sprintf("provider_installation {\n filesystem_mirror {\n path = %q\n include = [\"registry.terraform.io/cmdennis/mxroute\"]\n }\n}\ndisable_checkpoint = true\n", mirror)
	if err := os.WriteFile(cliConfig, []byte(config), 0600); err != nil {
		t.Fatal("cannot configure isolated mirror")
	}
	for name, value := range map[string]string{"HTTPS_PROXY": proxy.URL, "HTTP_PROXY": proxy.URL, "ALL_PROXY": proxy.URL,
		"https_proxy": proxy.URL, "http_proxy": proxy.URL, "NO_PROXY": "", "no_proxy": "", "SSL_CERT_FILE": ca,
		"TF_CLI_CONFIG_FILE": cliConfig, "TF_REATTACH_PROVIDERS": "", "TF_PLUGIN_CACHE_DIR": "", "CHECKPOINT_DISABLE": "1",
		"MXROUTE_SERVER": "mail.example.com", "MXROUTE_USERNAME": "owner", "MXROUTE_API_KEY": key, "TF_VAR_fixture_password": password} {
		t.Setenv(name, value)
	}
	cli := os.Getenv("TF_ACC_TERRAFORM_PATH")
	if cli == "" {
		cli, err = exec.LookPath("terraform")
		if err != nil {
			t.Fatal("install Terraform; release smoke tests never download it")
		}
	}
	run := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cli, args...)
		cmd.Dir = dir
		out, runErr := cmd.CombinedOutput()
		for _, secret := range checks.secrets {
			if bytes.Contains(out, []byte(secret)) {
				t.Fatal("release binary exposed a runtime secret in CLI output; output withheld")
			}
		}
		if runErr != nil {
			t.Fatalf("isolated Terraform %s failed: %s", args[0], out)
		}
		if err := checks.inspectDisk(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	writeConfig := func(quota, version int, withPassword bool) {
		t.Helper()
		hcl := mailboxConfig("release", quota, 120, version, withPassword)
		hcl = strings.Replace(hcl, `source = "registry.terraform.io/cmdennis/mxroute"`, fmt.Sprintf(`source = "registry.terraform.io/cmdennis/mxroute", version = %q`, match[1]), 1)
		if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(hcl), 0600); err != nil {
			t.Fatal("cannot write synthetic configuration")
		}
	}
	noChanges := func() {
		t.Helper()
		run("plan", "-input=false", "-detailed-exitcode", "-out=no-change.tfplan")
		var plan struct {
			ResourceChanges []struct{ Change struct{ Actions []string } } `json:"resource_changes"`
		}
		if err := json.Unmarshal(run("show", "-json", "no-change.tfplan"), &plan); err != nil {
			t.Fatal("cannot inspect saved release plan")
		}
		for _, change := range plan.ResourceChanges {
			if strings.Join(change.Change.Actions, ",") != "no-op" {
				t.Fatal("release lifecycle produced a nonempty plan")
			}
		}
	}
	writeConfig(128, 1, true)
	run("init", "-backend=false", "-input=false")
	run("validate")
	if runtime.GOOS != "linux" {
		// Linux honors SSL_CERT_FILE for an isolated test CA. macOS and Windows
		// use system trust stores; never change the user's trust store for a test.
		if os.Getenv("MXROUTE_REQUIRE_RELEASE_LIFECYCLE") == "true" {
			t.Fatal("full release lifecycle requires Linux with an isolated SSL_CERT_FILE")
		}
		t.Log("Packaged production binary installed and schema validated. Full mock lifecycle requires Linux; no system trust settings changed.")
		return
	}
	run("plan", "-input=false", "-out=create.tfplan")
	run("apply", "-input=false", "create.tfplan")
	writeConfig(256, 2, true)
	rotated := testutil.Secret(t)
	checks.secrets = append(checks.secrets, rotated)
	t.Setenv("TF_VAR_fixture_password", rotated)
	s.mu.Lock()
	s.password = rotated
	s.mu.Unlock()
	run("plan", "-input=false", "-out=update.tfplan")
	run("apply", "-input=false", "update.tfplan")
	noChanges()
	run("state", "rm", "mxroute_email_account.test")
	writeConfig(256, 0, false)
	run("import", "-input=false", "mxroute_email_account.test", "example.com/release")
	noChanges()
	run("plan", "-destroy", "-input=false", "-out=delete.tfplan")
	run("apply", "-input=false", "delete.tfplan")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.rotations != 2 || len(s.patches) != 1 || s.deletes != 1 || len(s.accounts) != 0 {
		t.Fatal("released binary did not complete the exact synthetic lifecycle/import workflow")
	}
}
