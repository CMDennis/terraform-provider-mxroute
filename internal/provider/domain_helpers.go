package provider

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type domainValidator struct{}

func (domainValidator) Description(context.Context) string {
	return "Canonical lowercase ASCII DNS name without a trailing dot."
}
func (v domainValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (domainValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v := req.ConfigValue.ValueString()
	canonical, err := identity.Domain(v)
	if err != nil || canonical != v {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid domain name", "Use a canonical lowercase ASCII DNS name without a trailing dot; convert international names to punycode.")
	}
}

type waitValidator struct{}

func (waitValidator) Description(context.Context) string               { return "Whole seconds from 0 through 1800." }
func (v waitValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (waitValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && (req.ConfigValue.ValueInt64() < 0 || req.ConfigValue.ValueInt64() > 1800) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid wait", "Use a whole number of seconds from 0 through 1800.")
	}
}

type modeValidator struct{}

func (modeValidator) Description(context.Context) string               { return "alias or redirect" }
func (v modeValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (modeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && req.ConfigValue.ValueString() != "alias" && req.ConfigValue.ValueString() != "redirect" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid pointer type", "Use alias or redirect.")
	}
}
func configuredClient(data any, dest **client.Client, d *diag.Diagnostics) {
	if data == nil {
		return
	}
	c, ok := data.(*client.Client)
	if !ok {
		d.AddError("Unexpected provider client", "Expected an MXroute client.")
		return
	}
	*dest = c
}
func apiDiagnostic(d *diag.Diagnostics, err error) {
	d.AddError("MXroute operation failed", err.Error())
}
func protocolError() error { return &client.Error{Operation: "readback", Kind: client.Protocol} }
func isMissingResponse(err error) bool {
	var e *client.Error
	return errors.As(err, &e) && e.Kind == client.NotFound
}

// A detail 404 alone is insufficient evidence to discard state.
func readDomain(ctx context.Context, c *client.Client, name string) (*client.Domain, error) {
	v, err := c.GetDomain(ctx, name)
	if err != nil {
		if isMissingResponse(err) {
			if _, lookup := c.FindDomain(ctx, name); lookup != nil {
				return nil, lookup
			}
		}
		return nil, err
	}
	canonical, e := identity.Domain(*v.Domain)
	if e != nil || canonical != name {
		return nil, protocolError()
	}
	return v, nil
}
func readPointer(ctx context.Context, c *client.Client, domain, pointer string) (*client.DomainPointer, error) {
	// An unavailable or deleted parent does not by itself prove a pointer
	// absent: domain deletion cascades are unverified. Require a complete child
	// list even after confirmed parent deletion; a child-route 404 is an error.
	_, parentErr := readDomain(ctx, c, domain)
	if parentErr != nil && !client.IsNotFound(parentErr) {
		return nil, parentErr
	}
	p, err := c.FindDomainPointer(ctx, domain, pointer)
	if err != nil {
		return nil, err
	}
	if parentErr != nil {
		return nil, protocolError()
	}
	canonical, e := identity.Domain(*p.Pointer)
	target, e2 := identity.Domain(*p.Target)
	if e != nil || e2 != nil || canonical != pointer || target != domain || (*p.Type != "alias" && *p.Type != "redirect") {
		return nil, protocolError()
	}
	return p, nil
}
func domainValues(ctx context.Context, m *domainModel, v *client.Domain, d *diag.Diagnostics) {
	m.MailHosting = types.BoolValue(*v.MailHosting)
	m.MailHostingPending = types.BoolValue(false)
	m.SSLEnabled = types.BoolValue(*v.SSLEnabled)
	var ds diag.Diagnostics
	m.Pointers, ds = types.SetValueFrom(ctx, types.StringType, v.Pointers)
	d.Append(ds...)
}

// Poll only a specifically recognized condition. Every other error returns
// immediately. Deadlines also bound HTTP requests and DNS lookups.
func poll(ctx context.Context, seconds int64, check func(context.Context) (bool, error)) error {
	if seconds == 0 {
		done, err := check(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		return errors.New("MXroute readiness condition is not yet satisfied; increase the configured wait")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	for {
		done, err := check(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.New("MXroute readiness wait expired; preserve state and retry after checking readiness")
		case <-timer.C:
		}
	}
}

var lookupTXT = net.DefaultResolver.LookupTXT

func waitVerification(ctx context.Context, c *client.Client, domain string, seconds int64) error {
	if seconds == 0 {
		return nil
	}
	key, err := c.GetVerificationKey(ctx)
	if err != nil {
		return err
	}
	if *key.Record.Type != "TXT" {
		return protocolError()
	}
	name := strings.TrimSuffix(*key.Record.Name, ".") + "." + domain
	return poll(ctx, seconds, func(ctx context.Context) (bool, error) {
		records, err := lookupTXT(ctx, name)
		if err != nil {
			var dns *net.DNSError
			if errors.As(err, &dns) && dns.IsNotFound {
				return false, nil
			}
			return false, errors.New("DNS verification lookup failed; check the resolver and retry")
		}
		for _, value := range records {
			if value == *key.Record.Value {
				return true, nil
			}
		}
		return false, nil
	})
}
func waitDomain(ctx context.Context, c *client.Client, name string, seconds int64) (*client.Domain, error) {
	var result *client.Domain
	err := poll(ctx, seconds, func(ctx context.Context) (bool, error) {
		v, err := readDomain(ctx, c, name)
		if client.IsNotFound(err) {
			return false, nil
		}
		result = v
		return err == nil, err
	})
	return result, err
}
func waitPointer(ctx context.Context, c *client.Client, domain, pointer string, seconds int64) (*client.DomainPointer, error) {
	var result *client.DomainPointer
	err := poll(ctx, seconds, func(ctx context.Context) (bool, error) {
		v, err := readPointer(ctx, c, domain, pointer)
		if client.IsNotFound(err) {
			return false, nil
		}
		result = v
		return err == nil, err
	})
	return result, err
}
