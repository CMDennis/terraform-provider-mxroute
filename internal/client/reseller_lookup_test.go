package client

import "testing"

func TestResellerExactLookup(t *testing.T) {
	for _, c := range []struct {
		items []string
		name  string
		kind  ErrorKind
	}{
		{[]string{"starter", "Premium-2"}, "Premium-2", ""},
		{[]string{"starter"}, "Premium-2", NotFound},
		{[]string{}, "starter", NotFound},
		{[]string{"STARTER"}, "starter", Ambiguous},
		{[]string{"starter", "Starter"}, "starter", Ambiguous},
		{[]string{"starter", "starter"}, "starter", Ambiguous},
	} {
		err := exactName(c.items, c.name)
		if c.kind == "" {
			if err != nil {
				t.Fatal("exact match was not found")
			}
			continue
		}
		e, ok := err.(*Error)
		if !ok || e.Kind != c.kind || IsNotFound(err) != (c.kind == NotFound) {
			t.Fatalf("lookup %v for %q: want %s", c.items, c.name, c.kind)
		}
	}
}
