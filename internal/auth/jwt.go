package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

// Claims JWT 载荷
type Claims struct {
	UserID     string `json:"user_id"`
	TenantID   uint   `json:"tenant_id"`
	TenantCode string `json:"tenant_code"`
	Username   string `json:"username"`
	jwt.RegisteredClaims
}

// TokenManager JWT 签发与校验
type TokenManager struct {
	secret []byte
	expire time.Duration
	issuer string
}

func NewTokenManager(cfg config.JWTConfig) *TokenManager {
	return &TokenManager{
		secret: []byte(cfg.Secret),
		expire: time.Duration(cfg.ExpireHours) * time.Hour,
		issuer: cfg.Issuer,
	}
}

func (m *TokenManager) Sign(userID, username, tenantCode string, tenantID uint) (string, time.Time, error) {
	expiresAt := time.Now().Add(m.expire)
	claims := Claims{
		UserID:     userID,
		TenantID:   tenantID,
		TenantCode: tenantCode,
		Username:   username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, expiresAt, nil
}

func (m *TokenManager) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.UserID == "" {
		return nil, fmt.Errorf("token missing user_id")
	}
	return claims, nil
}
