package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

func TestMockTransportAndRedaction(t *testing.T) {
	testutil.ClearCredentials(t)
	key := testutil.Secret(t) + ":" + testutil.Secret(t)
	called := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("X-Server") != "mail.example.com" || r.Header.Get("X-Username") != "owner|ordinary" || r.Header.Get("X-API-Key") != key || r.UserAgent() != "terraform-provider-mxroute/test" {
			t.Error("configured headers changed")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s.Close()
	c, err := NewMock("mail.example.com", "owner|ordinary", key, "test", s.URL)
	if err != nil {
		t.Fatal("mock client initialization failed")
	}
	if c.RequireReseller() == nil || c.Account().Owner != "owner" || c.Account().Effective != "ordinary" {
		t.Fatal("lost impersonation boundary")
	}
	for _, rendered := range []string{fmt.Sprint(c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(rendered, key) || strings.Contains(rendered, "mail.example.com") || strings.Contains(rendered, "owner") {
			t.Fatal("client formatting leaked configuration")
		}
	}
	resp, err := c.Request(context.Background(), http.MethodGet, identity.Path("verification-key"), nil)
	if err != nil {
		t.Fatal("loopback request failed")
	}
	_ = resp.Body.Close()
	if !called {
		t.Fatal("mock was not used")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Request(ctx, http.MethodGet, "/cancel", nil); err != context.Canceled {
		t.Fatal("cancellation was not preserved")
	}
}

func TestMockFailsClosed(t *testing.T) {
	key := testutil.Secret(t)
	for _, endpoint := range []string{"", ProductionURL, "https://127.0.0.1:1234", "http://localhost:1234", "http://192.0.2.1:1234", "http://127.0.0.1:1234/path", "http://127.0.0.1:1234?redirect=yes", "http://127.0.0.1"} {
		if _, err := NewMock("mail.example.com", "ordinary", key, "test", endpoint); err == nil {
			t.Fatal("unsafe mock endpoint accepted")
		}
	}
}

func TestRedirectNeverCarriesCredentials(t *testing.T) {
	key := testutil.Secret(t)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("followed credential-bearing redirect") }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	c, err := NewMock("mail.example.com", "ordinary", key, "test", source.URL)
	if err != nil {
		t.Fatal("mock initialization failed")
	}
	resp, err := c.Request(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal("redirect response unavailable")
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatal("redirect was not preserved for operation-level rejection")
	}
	for _, path := range []string{"https://example.com", "//example.com", "/?x=y", "/#fragment", "/\n"} {
		if _, err := c.Request(context.Background(), http.MethodGet, path, nil); err == nil {
			t.Fatal("unsafe path accepted")
		}
	}
}
