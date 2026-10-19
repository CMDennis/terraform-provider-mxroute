package identity

import "testing"

func TestAccountBoundaries(t *testing.T) {
	for _, name := range []string{"", "owner|", "|user", "owner|user|extra", "owner|owner", "two words", "longer_than_16_characters", "user\n"} {
		if _, err := ParseAccount(name); err == nil {
			t.Fatal("accepted invalid authentication identity")
		}
	}
	a, err := ParseAccount("Owner_1|User_2")
	if err != nil || a.Owner != "Owner_1" || a.Effective != "User_2" || !a.Impersonated {
		t.Fatal("lost account boundary or configured case")
	}
	if ValidateNewUser("Owner_1") == nil || ValidateNewUser("ordinary") != nil || ValidateNewUser("user_1") != nil {
		t.Fatal("authentication and creation constraints conflated")
	}
	if ExistingUser("Owner_1") != nil || ExistingUser("owner|user") == nil || ExistingUser("longer_than_16_characters") == nil || ExistingUser("") == nil {
		t.Fatal("existing reseller users must use plain authentication syntax")
	}
}

func TestAccountScopedImportKeys(t *testing.T) {
	if name, err := Package("Mail_Pro-1"); err != nil || name != "Mail_Pro-1" {
		t.Fatal("package case was not preserved")
	}
	if _, err := Package("../package"); err == nil {
		t.Fatal("package path accepted")
	}
	if entry, err := Entry("*@EXAMPLE.NET"); err != nil || entry != "*@example.net" {
		t.Fatal("spam comparison key was not canonicalized")
	}
	if _, err := Entry("example.com/*@example.net"); err == nil {
		t.Fatal("access domain leaked into shared entry identity")
	}
}

func TestImportAndPathConventions(t *testing.T) {
	d, n, err := Scoped("EXAMPLE.COM./MailBox", false)
	if err != nil || d != "example.com" || n != "mailbox" {
		t.Fatal("mailbox import normalization failed")
	}
	d, n, err = Scoped("EXAMPLE.COM/Alias.EXAMPLE.NET", true)
	if err != nil || d != "example.com" || n != "alias.example.net" {
		t.Fatal("pointer import normalization failed")
	}
	for _, id := range []string{"example.com", "example.com/a/b", "example.com/", "example.com/a@example.com", "example.com/..", "https://example.com/a"} {
		if _, _, err := Scoped(id, false); err == nil {
			t.Fatal("accepted invalid scoped import")
		}
	}
	if got := Path("domains", "example.com", "whitelist", "*@example.net/a%25"); got != "/domains/example.com/whitelist/%2A@example.net%2Fa%2525" {
		t.Fatal("path segments were not escaped exactly once")
	}
	for _, host := range []string{"https://mail.example.com", "mail.example.com:443", "mail.example.com/path", "mail.example.com\n", ""} {
		if Server(host) == nil {
			t.Fatal("accepted invalid server hostname")
		}
	}
}
