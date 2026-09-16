package service

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

var (
	// ErrTenantNotFound 租户 ID 不存在
	ErrTenantNotFound = errors.New("租户 ID 不存在，请确认后重试")
	// ErrInvalidCredentials 用户名或密码错误
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	// ErrUsernameTaken 租户内用户名已占用
	ErrUsernameTaken = errors.New("该租户下用户名已存在")
	// ErrTenantCodeTaken 租户 Code 已占用
	ErrTenantCodeTaken = errors.New("租户 ID 已存在")
)

type AuthService struct {
	tenants *repository.TenantRepo
	users   *repository.UserRepo
	tokens  *auth.TokenManager
}

func NewAuthService(
	tenants *repository.TenantRepo,
	users *repository.UserRepo,
	tokens *auth.TokenManager,
) *AuthService {
	return &AuthService{tenants: tenants, users: users, tokens: tokens}
}

type CreateTenantInput struct {
	Code string
	Name string
}

func (s *AuthService) CreateTenant(in CreateTenantInput) (*model.Tenant, error) {
	code := strings.TrimSpace(in.Code)
	name := strings.TrimSpace(in.Name)
	if code == "" {
		return nil, fmt.Errorf("租户 ID 不能为空")
	}
	if utf8.RuneCountInString(code) > 64 {
		return nil, fmt.Errorf("租户 ID 过长")
	}
	if name == "" {
		name = code
	}
	if _, err := s.tenants.GetByCode(code); err == nil {
		return nil, ErrTenantCodeTaken
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	t := &model.Tenant{Code: code, Name: name}
	if err := s.tenants.Create(t); err != nil {
		return nil, err
	}
	return t, nil
}

type RegisterInput struct {
	TenantCode  string
	Username    string
	Password    string
	DisplayName string
}

type AuthResult struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      AuthUserView `json:"user"`
	Tenant    TenantView   `json:"tenant"`
}

type AuthUserView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type TenantView struct {
	ID   uint   `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

func (s *AuthService) requireTenantByCode(code string) (*model.Tenant, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("租户 ID 不能为空")
	}
	t, err := s.tenants.GetByCode(code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *AuthService) Register(in RegisterInput) (*AuthResult, error) {
	tenant, err := s.requireTenantByCode(in.TenantCode)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(in.Username)
	password := in.Password
	if username == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}
	if utf8.RuneCountInString(password) < 6 {
		return nil, fmt.Errorf("密码至少 6 位")
	}

	if _, err := s.users.GetByTenantUsername(tenant.ID, username); err == nil {
		return nil, ErrUsernameTaken
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = username
	}
	user := &model.User{
		TenantID:     tenant.ID,
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  display,
	}
	if err := s.users.Create(user); err != nil {
		return nil, err
	}
	return s.issueAuth(user, tenant)
}

type LoginInput struct {
	TenantCode string
	Username   string
	Password   string
}

func (s *AuthService) Login(in LoginInput) (*AuthResult, error) {
	tenant, err := s.requireTenantByCode(in.TenantCode)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(in.Username)
	if username == "" || in.Password == "" {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.GetByTenantUsername(tenant.ID, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.issueAuth(user, tenant)
}

func (s *AuthService) Me(userID string) (*AuthResult, error) {
	// userID 为数字字符串
	var id uint64
	if _, err := fmt.Sscanf(userID, "%d", &id); err != nil || id == 0 {
		return nil, fmt.Errorf("无效用户")
	}
	user, err := s.users.GetByID(uint(id))
	if err != nil {
		return nil, fmt.Errorf("用户不存在")
	}
	tenant, err := s.tenants.GetByID(user.TenantID)
	if err != nil {
		return nil, ErrTenantNotFound
	}
	return &AuthResult{
		User: AuthUserView{
			ID:          user.AuthUserID(),
			Username:    user.Username,
			DisplayName: user.DisplayName,
		},
		Tenant: TenantView{
			ID:   tenant.ID,
			Code: tenant.Code,
			Name: tenant.Name,
		},
	}, nil
}

func (s *AuthService) issueAuth(user *model.User, tenant *model.Tenant) (*AuthResult, error) {
	token, exp, err := s.tokens.Sign(user.AuthUserID(), user.Username, tenant.Code, tenant.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResult{
		Token:     token,
		ExpiresAt: exp,
		User: AuthUserView{
			ID:          user.AuthUserID(),
			Username:    user.Username,
			DisplayName: user.DisplayName,
		},
		Tenant: TenantView{
			ID:   tenant.ID,
			Code: tenant.Code,
			Name: tenant.Name,
		},
	}, nil
}
