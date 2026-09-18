package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

const (
	defaultTenantCode   = "default"
	tenantAdminUsername = "admin"
	adminPasswordLen    = 16
	passwordAlphabet    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// IsPlatformAdmin 是否为 default 租户下的 admin（唯一可创建租户的账号）。
func IsPlatformAdmin(tenantCode, username string) bool {
	return strings.TrimSpace(tenantCode) == defaultTenantCode &&
		strings.TrimSpace(username) == tenantAdminUsername
}

// EnsureTenantAdmin 确保租户下存在 admin。已存在则跳过，不重置密码。
func (s *AuthService) EnsureTenantAdmin(tenant *model.Tenant) error {
	if tenant == nil || tenant.ID == 0 {
		return fmt.Errorf("无效租户")
	}
	created, password, err := ensureTenantAdmin(s.users, tenant)
	if err != nil {
		return err
	}
	if created {
		logTenantAdminCreated(tenant, password)
	}
	return nil
}

func ensureTenantAdmin(users *repository.UserRepo, tenant *model.Tenant) (created bool, password string, err error) {
	if _, err = users.GetByTenantUsername(tenant.ID, tenantAdminUsername); err == nil {
		return false, "", nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "", err
	}

	password, err = randomPassword(adminPasswordLen)
	if err != nil {
		return false, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, "", fmt.Errorf("hash password: %w", err)
	}
	user := &model.User{
		TenantID:     tenant.ID,
		Username:     tenantAdminUsername,
		PasswordHash: string(hash),
		DisplayName:  "管理员",
	}
	if err = users.Create(user); err != nil {
		if _, getErr := users.GetByTenantUsername(tenant.ID, tenantAdminUsername); getErr == nil {
			return false, "", nil
		}
		return false, "", err
	}
	return true, password, nil
}

func randomPassword(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("invalid password length")
	}
	alphabet := []rune(passwordAlphabet)
	if len(alphabet) == 0 {
		return "", fmt.Errorf("empty password alphabet")
	}
	max := big.NewInt(int64(len(alphabet)))
	out := make([]rune, n)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[v.Int64()]
	}
	return string(out), nil
}

func logTenantAdminCreated(tenant *model.Tenant, password string) {
	logger.L().Info("租户管理员已创建",
		zap.Uint("tenant_id", tenant.ID),
		zap.String("tenant_code", tenant.Code),
		zap.String("username", tenantAdminUsername),
		zap.String("password", password),
	)
}

func newCreateTenantResult(tenant *model.Tenant) *CreateTenantResult {
	return &CreateTenantResult{
		ID:            tenant.ID,
		Code:          tenant.Code,
		Name:          tenant.Name,
		CreatedAt:     tenant.CreatedAt,
		UpdatedAt:     tenant.UpdatedAt,
		AdminUsername: tenantAdminUsername,
	}
}
