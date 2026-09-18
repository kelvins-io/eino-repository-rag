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
	// ErrNotPlatformAdmin 非平台管理员，无权创建租户
	ErrNotPlatformAdmin = errors.New("仅 default 租户的 admin 可创建租户")
	// ErrNotTenantAdmin 非本租户管理员
	ErrNotTenantAdmin = errors.New("仅租户管理员可修改登录权限")
	// ErrLoginDisabled 账号已被禁止登录
	ErrLoginDisabled = errors.New("该账号已被禁止登录")
	// ErrCannotDisableAdmin 不能关闭租户管理员登录
	ErrCannotDisableAdmin = errors.New("不能关闭租户管理员的登录权限")
	// ErrRegisterNotAllowed 该租户不允许自助注册
	ErrRegisterNotAllowed = errors.New("当前租户不允许注册新用户")
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
	Code            string
	Name            string
	ActorTenantCode string
	ActorUsername   string
}

// CreateTenantResult 创建租户的返回。初始密码只写日志，不出现在响应里。
type CreateTenantResult struct {
	ID            uint      `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	AdminUsername string    `json:"admin_username"`
}

func (s *AuthService) CreateTenant(in CreateTenantInput) (*CreateTenantResult, error) {
	if !IsPlatformAdmin(in.ActorTenantCode, in.ActorUsername) {
		return nil, ErrNotPlatformAdmin
	}
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

	var tenant *model.Tenant
	var password string
	var createdAdmin bool
	err := s.tenants.Transaction(func(tenants *repository.TenantRepo, users *repository.UserRepo) error {
		t := &model.Tenant{Code: code, Name: name}
		if err := tenants.Create(t); err != nil {
			return err
		}
		created, pwd, err := ensureTenantAdmin(users, t)
		if err != nil {
			return err
		}
		tenant = t
		password = pwd
		createdAdmin = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	if createdAdmin {
		logTenantAdminCreated(tenant, password)
	}
	return newCreateTenantResult(tenant), nil
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
	if strings.EqualFold(strings.TrimSpace(in.TenantCode), defaultTenantCode) {
		return nil, ErrRegisterNotAllowed
	}
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
		LoginEnabled: true,
	}
	if err := s.users.Create(user); err != nil {
		return nil, err
	}
	return s.issueAuth(user, tenant)
}

// TenantUserView 租户用户列表项，不包含密码等敏感字段。
type TenantUserView struct {
	Username     string    `json:"username"`
	CreatedAt    time.Time `json:"created_at"`
	IsAdmin      bool      `json:"is_admin"`
	LoginEnabled bool      `json:"login_enabled"`
}

// ListTenantUsers 分页列出当前租户下的用户。用户名为 admin 的账号视为租户管理员。
func (s *AuthService) ListTenantUsers(tenantID uint, page, pageSize int) ([]TenantUserView, int64, error) {
	if tenantID == 0 {
		return nil, 0, fmt.Errorf("无效租户")
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	users, total, err := s.users.ListByTenant(tenantID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]TenantUserView, 0, len(users))
	for _, u := range users {
		out = append(out, TenantUserView{
			Username:     u.Username,
			CreatedAt:    u.CreatedAt,
			IsAdmin:      u.Username == tenantAdminUsername,
			LoginEnabled: u.LoginEnabled,
		})
	}
	return out, total, nil
}

// SetUserLoginEnabled 由租户 admin 开关本租户用户的登录权限。不能关闭 admin 自己。
func (s *AuthService) SetUserLoginEnabled(tenantID uint, actorUsername, targetUsername string, enabled bool) error {
	if tenantID == 0 {
		return fmt.Errorf("无效租户")
	}
	if strings.TrimSpace(actorUsername) != tenantAdminUsername {
		return ErrNotTenantAdmin
	}
	targetUsername = strings.TrimSpace(targetUsername)
	if targetUsername == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if targetUsername == tenantAdminUsername && !enabled {
		return ErrCannotDisableAdmin
	}
	if _, err := s.users.GetByTenantUsername(tenantID, targetUsername); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在")
		}
		return err
	}
	return s.users.SetLoginEnabled(tenantID, targetUsername, enabled)
}

// EnsureLoginAllowed 已登录请求校验账号仍允许登录。
func (s *AuthService) EnsureLoginAllowed(userID string) error {
	var id uint64
	if _, err := fmt.Sscanf(userID, "%d", &id); err != nil || id == 0 {
		return fmt.Errorf("无效用户")
	}
	user, err := s.users.GetByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("用户不存在")
		}
		return err
	}
	if !user.LoginEnabled {
		return ErrLoginDisabled
	}
	return nil
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
	if !user.LoginEnabled {
		return nil, ErrLoginDisabled
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
