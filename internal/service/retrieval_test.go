package service

import "testing"

func TestNormalizeLabelDocIDs(t *testing.T) {
	got, err := normalizeLabelDocIDs([]string{" 12 ", "12", "3", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "12" || got[1] != "3" {
		t.Fatalf("%v", got)
	}
	if _, err := normalizeLabelDocIDs([]string{"abc"}); err == nil {
		t.Fatal("expected invalid doc id")
	}
}

func TestRequireTenantAdmin(t *testing.T) {
	if err := requireTenantAdmin("admin"); err != nil {
		t.Fatal(err)
	}
	if err := requireTenantAdmin(" admin "); err != nil {
		t.Fatal(err)
	}
	if err := requireTenantAdmin("voiceqa"); err == nil {
		t.Fatal("expected non-admin rejected")
	}
}
