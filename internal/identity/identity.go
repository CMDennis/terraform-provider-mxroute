// Package identity contains account boundaries and reversible Terraform import
// identifiers. It has no dependency on Terraform or HTTP transport.
package identity

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var authName = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)
var newUserName = regexp.MustCompile(`^[a-z0-9_]{1,10}$`)
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var packageName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var spamEntry = regexp.MustCompile(`^[A-Za-z0-9@.\-_*+=]+$`)

// Account distinguishes the credential owner from the account being managed.
// Authentication names retain their configured case when sent to the API.
type Account struct {
	Owner, Effective string
	Impersonated     bool
}

func ParseAccount(username string) (Account, error) {
	parts := strings.Split(username, "|")
	if len(parts) > 2 || !authName.MatchString(parts[0]) || (len(parts) == 2 && !authName.MatchString(parts[1])) {
		return Account{}, errors.New("use a 1–16 character name containing letters, digits or underscores, or two such names separated by one pipe")
	}
	a := Account{Owner: parts[0], Effective: parts[0]}
	if len(parts) == 2 {
		if strings.EqualFold(parts[0], parts[1]) {
			return Account{}, errors.New("use a plain username for your own account; impersonation requires a distinct target")
		}
		a.Effective, a.Impersonated = parts[1], true
	}
	return a, nil
}

// ValidateNewUser is intentionally stricter than authentication syntax.
func ValidateNewUser(name string) error {
	if !newUserName.MatchString(name) {
		return errors.New("new reseller users require 1–10 lowercase letters, digits or underscores")
	}
	return nil
}

// ExistingUser accepts an existing reseller-owned username using the plain
// authentication syntax. Creation separately applies ValidateNewUser.
func ExistingUser(name string) error {
	if !authName.MatchString(name) {
		return errors.New("use an existing 1–16 character username containing letters, digits or underscores")
	}
	return nil
}

// Package preserves case because the API does not promise case-insensitive
// package lookup. It validates only the published character restriction.
func Package(value string) (string, error) {
	if !packageName.MatchString(value) {
		return "", errors.New("use a nonempty package name containing letters, digits, underscores or hyphens")
	}
	return value, nil
}

// Entry identifies an account-shared spam entry independently of its domain
// access context. This is a comparison key, not an assumption about wire case.
func Entry(value string) (string, error) {
	if !spamEntry.MatchString(value) {
		return "", errors.New("use the documented spam entry character set without spaces or separators")
	}
	return strings.ToLower(value), nil
}

func Domain(value string) (string, error) {
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if len(value) > 253 || !strings.Contains(value, ".") {
		return "", errors.New("use an ASCII DNS domain name with at least two labels")
	}
	for _, label := range strings.Split(value, ".") {
		if !dnsLabel.MatchString(label) {
			return "", errors.New("use an ASCII DNS domain name; convert international names to punycode")
		}
	}
	return value, nil
}

func Server(value string) error {
	_, err := Domain(value)
	if err != nil || strings.HasSuffix(value, ".") {
		return errors.New("use a mail server hostname without a scheme, port, path or trailing dot")
	}
	return nil
}

// Scoped is the domain/name import form for mailboxes, pointers and forwarders.
// Package names retain case; DNS names and case-insensitive mailbox/alias keys
// use separate normalization rules rather than lowercasing every identifier.
func Scoped(value string, dnsName bool) (domain, name string, err error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return "", "", errors.New("use domain/name as the import identifier")
	}
	domain, err = Domain(parts[0])
	if err != nil {
		return "", "", err
	}
	name = parts[1]
	if dnsName {
		name, err = Domain(name)
	} else {
		name = strings.ToLower(name)
		if name == "" || strings.ContainsAny(name, "@|\\ \t\r\n") || name == "." || name == ".." {
			err = errors.New("use a nonempty local name without an address domain or separator")
		}
	}
	return domain, name, err
}

// Path escapes each raw segment exactly once, including slash, percent and
// wildcard characters. Callers must never pass pre-escaped identifiers.
func Path(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return "/" + strings.Join(escaped, "/")
}
