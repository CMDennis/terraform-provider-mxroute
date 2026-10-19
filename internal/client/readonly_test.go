package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestReadOnlyTransportBlocksAllMutationsAndUnknownRoutes(t *testing.T) {
	calls := 0
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}, "owner")
	c.http.Transport = &readOnlyTransport{origin: c.baseURL, next: c.http.Transport}
	for _, method := range []string{"POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS"} {
		if _, err := c.Request(context.Background(), method, "/domains/example.com", nil); err == nil {
			t.Fatal("read-only transport accepted a non-GET operation")
		}
	}
	for _, path := range []string{"/reseller/users", "/reseller/packages", "/domains/example.com/mail-status", "/domains/example.com/spam/unknown", "/domains/example.com/../quota", "/domains/example.com/email-accounts/%2Fescape", "/domains/example.com/forwarders/alias", "/unknown"} {
		if _, err := c.Request(context.Background(), "GET", path, nil); err == nil {
			t.Fatal("read-only transport accepted an unapproved route")
		}
	}
	if calls != 0 {
		t.Fatal("blocked discovery operation reached the service")
	}
	for _, path := range []string{"/domains", "/quota", "/quota/email", "/verification-key", "/domains/example.com", "/domains/example.com/pointers", "/domains/example.com/email-accounts", "/domains/example.com/email-accounts/support", "/domains/example.com/forwarders", "/domains/example.com/catch-all", "/domains/example.com/dns", "/domains/example.com/spam/settings", "/domains/example.com/spam/whitelist", "/domains/example.com/spam/blacklist"} {
		resp, err := c.Request(context.Background(), "GET", path, nil)
		if err != nil {
			t.Fatal("approved discovery read was blocked")
		}
		_ = resp.Body.Close()
	}
	if calls != 14 {
		t.Fatal("unexpected discovery request count")
	}
}

func TestReadOnlyTransportRejectsOriginBodyAndQuery(t *testing.T) {
	guard := &readOnlyTransport{origin: ProductionURL}
	request, err := http.NewRequest("GET", ProductionURL+"/domains", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal("cannot construct synthetic request body")
	}
	if _, err := guard.RoundTrip(request); err == nil {
		t.Fatal("discovery guard accepted a request body")
	}
	for _, address := range []string{"https://example.com/domains", "http://api.mxroute.com/domains", ProductionURL + "/domains?", ProductionURL + "/domains?test=1", ProductionURL + "/domains#fragment", "https://owner@api.mxroute.com/domains"} {
		r, err := http.NewRequest("GET", address, nil)
		if err != nil {
			t.Fatal("invalid synthetic request")
		}
		if _, err := guard.RoundTrip(r); err == nil {
			t.Fatal("discovery guard accepted a different origin or URL metadata")
		}
	}
}
