package service

import (
	"testing"
	"unicode/utf8"
)

func TestRandomPassword(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		p, err := randomPassword(adminPasswordLen)
		if err != nil {
			t.Fatal(err)
		}
		if utf8.RuneCountInString(p) != adminPasswordLen {
			t.Fatalf("len=%d password=%q", utf8.RuneCountInString(p), p)
		}
		for _, r := range p {
			if !containsRune(passwordAlphabet, r) {
				t.Fatalf("unexpected rune %q in %q", r, p)
			}
		}
		seen[p] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatal("expected distinct passwords")
	}
}

func TestIsPlatformAdmin(t *testing.T) {
	cases := []struct {
		tenant, user string
		want         bool
	}{
		{"default", "admin", true},
		{" default ", " admin ", true},
		{"default", "other", false},
		{"acme", "admin", false},
		{"", "admin", false},
		{"default", "", false},
	}
	for _, tc := range cases {
		if got := IsPlatformAdmin(tc.tenant, tc.user); got != tc.want {
			t.Fatalf("IsPlatformAdmin(%q,%q)=%v want %v", tc.tenant, tc.user, got, tc.want)
		}
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
