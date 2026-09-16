package auth

import (
	"testing"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

func TestTokenSignParse(t *testing.T) {
	tm := NewTokenManager(config.JWTConfig{
		Secret:      "test-secret",
		ExpireHours: 1,
		Issuer:      "test",
	})
	token, exp, err := tm.Sign("42", "alice", "default", 1)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if time.Until(exp) < 30*time.Minute {
		t.Fatalf("unexpected expiry %v", exp)
	}
	claims, err := tm.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "42" || claims.Username != "alice" || claims.TenantCode != "default" || claims.TenantID != 1 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenRejectTampered(t *testing.T) {
	tm := NewTokenManager(config.JWTConfig{Secret: "a", ExpireHours: 1, Issuer: "t"})
	token, _, err := tm.Sign("1", "u", "default", 1)
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokenManager(config.JWTConfig{Secret: "b", ExpireHours: 1, Issuer: "t"})
	if _, err := other.Parse(token); err == nil {
		t.Fatal("expected parse error with wrong secret")
	}
}
