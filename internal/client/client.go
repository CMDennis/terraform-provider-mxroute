// Package client owns HTTP configuration and API models independently of Terraform.
package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

const ProductionURL = "https://api.mxroute.com"

// Client contains credentials only in memory. Do not embed it in state models.
type Client struct {
	server, username, apiKey, version string
	account                           identity.Account
	baseURL                           string
	http                              *http.Client
	limits                            *accountLimits
	settings                          *settingLock
	operationTimeout, retryBase       time.Duration
}

func (c *Client) String() string   { return "MXroute client (credentials redacted)" }
func (c *Client) GoString() string { return c.String() }

func New(server, username, apiKey, version string) (*Client, error) {
	return newClient(server, username, apiKey, version, ProductionURL)
}

// NewMock requires an explicit numeric loopback HTTP address. It cannot fall
// back to production, resolve an arbitrary hostname, or inherit credentials.
// There is deliberately no provider endpoint argument or endpoint environment variable.
func NewMock(server, username, apiKey, version, endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() == "" {
		return nil, errors.New("mock endpoint must be an explicit loopback HTTP origin with a port")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("mock endpoint must use a numeric loopback address")
	}
	return newClient(server, username, apiKey, version, strings.TrimSuffix(endpoint, "/"))
}

func newClient(server, username, apiKey, version, endpoint string) (*Client, error) {
	if err := identity.Server(server); err != nil {
		return nil, err
	}
	a, err := identity.ParseAccount(username)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") {
		return nil, errors.New("supply a nonempty API key without line breaks")
	}
	if strings.ContainsAny(version, "\r\n") {
		return nil, errors.New("invalid provider version")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Tests must never use inherited proxy configuration to leave loopback.
	if endpoint != ProductionURL {
		transport.Proxy = nil
	}
	limits, settings := coordination(endpoint, server, a.Owner, a.Effective)
	return &Client{server: server, username: username, apiKey: apiKey, version: version, account: a, baseURL: endpoint,
		limits: limits, settings: settings, operationTimeout: 2 * time.Minute, retryBase: 500 * time.Millisecond,
		http: &http.Client{Timeout: 30 * time.Second, Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Account() identity.Account { return c.account }

// RequireReseller rejects privileged operations on impersonated configurations.
func (c *Client) RequireReseller() error {
	if c.account.Impersonated {
		return errors.New("reseller operations require a plain reseller provider configuration")
	}
	return nil
}

// Request is the single-attempt transport seam. Paths are built
// from raw segments by identity.Path; errors must never echo a request or body.
// Callers own closing response bodies. Typed operations add limits and decoding.
func (c *Client) Request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#\r\n") {
		return nil, errors.New("invalid API path")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, errors.New("unable to construct MXroute request")
	}
	req.Header.Set("X-Server", c.server)
	req.Header.Set("X-Username", c.username)
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", "terraform-provider-mxroute/"+c.version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("MXroute transport failed; request details redacted")
	}
	return resp, nil
}
