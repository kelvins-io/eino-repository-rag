package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kelvins-io/eino-repository-rag/internal/auth"
	"github.com/kelvins-io/eino-repository-rag/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type createTenantReq struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// CreateTenant POST /api/v1/tenants
func (h *AuthHandler) CreateTenant(c *gin.Context) {
	var req createTenantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	t, err := h.svc.CreateTenant(service.CreateTenantInput{
		Code:            req.Code,
		Name:            req.Name,
		ActorTenantCode: auth.TenantCodeFromContext(c),
		ActorUsername:   auth.UsernameFromContext(c),
	})
	if err != nil {
		failAuth(c, err)
		return
	}
	ok(c, t)
}

type registerReq struct {
	TenantID    string `json:"tenant_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// Register POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.Register(service.RegisterInput{
		TenantCode:  req.TenantID,
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		failAuth(c, err)
		return
	}
	ok(c, result)
}

type loginReq struct {
	TenantID string `json:"tenant_id"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.Login(service.LoginInput{
		TenantCode: req.TenantID,
		Username:   req.Username,
		Password:   req.Password,
	})
	if err != nil {
		failAuth(c, err)
		return
	}
	ok(c, result)
}

// Me GET /api/v1/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	result, err := h.svc.Me(auth.UserIDFromContext(c))
	if err != nil {
		failAuth(c, err)
		return
	}
	ok(c, result)
}

// ListUsers GET /api/v1/users
func (h *AuthHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.svc.ListTenantUsers(auth.TenantIDFromContext(c), page, pageSize)
	if err != nil {
		failAuth(c, err)
		return
	}
	ok(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

type setLoginEnabledReq struct {
	Username string `json:"username"`
	Enabled  *bool  `json:"enabled"`
}

// SetUserLoginEnabled PUT /api/v1/users/login-enabled
func (h *AuthHandler) SetUserLoginEnabled(c *gin.Context) {
	var req setLoginEnabledReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if req.Enabled == nil {
		fail(c, http.StatusBadRequest, "请求参数错误: enabled 不能为空")
		return
	}
	if err := h.svc.SetUserLoginEnabled(
		auth.TenantIDFromContext(c),
		auth.UsernameFromContext(c),
		req.Username,
		*req.Enabled,
	); err != nil {
		failAuth(c, err)
		return
	}
	ok(c, gin.H{
		"username":      strings.TrimSpace(req.Username),
		"login_enabled": *req.Enabled,
	})
}

// RequireLoginEnabled 拒绝已被关闭登录权限的已登录请求。
func (h *AuthHandler) RequireLoginEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := h.svc.EnsureLoginAllowed(auth.UserIDFromContext(c))
		if err == nil {
			c.Next()
			return
		}
		status := http.StatusUnauthorized
		msg := "登录已失效，请重新登录"
		if errors.Is(err, service.ErrLoginDisabled) {
			status = http.StatusForbidden
			msg = err.Error()
		}
		c.AbortWithStatusJSON(status, gin.H{
			"code":    status,
			"message": msg,
		})
	}
}

func failAuth(c *gin.Context, err error) {
	if err == nil {
		return
	}
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrTenantNotFound):
		code = http.StatusBadRequest
	case errors.Is(err, service.ErrInvalidCredentials):
		code = http.StatusUnauthorized
	case errors.Is(err, service.ErrNotPlatformAdmin),
		errors.Is(err, service.ErrNotTenantAdmin),
		errors.Is(err, service.ErrLoginDisabled),
		errors.Is(err, service.ErrRegisterNotAllowed):
		code = http.StatusForbidden
	case errors.Is(err, service.ErrCannotDisableAdmin):
		code = http.StatusBadRequest
	case errors.Is(err, service.ErrUsernameTaken), errors.Is(err, service.ErrTenantCodeTaken):
		code = http.StatusConflict
	case strings.Contains(err.Error(), "不能为空"),
		strings.Contains(err.Error(), "至少"),
		strings.Contains(err.Error(), "过长"),
		strings.Contains(err.Error(), "用户不存在"):
		code = http.StatusBadRequest
	}
	writeFail(c, code, err.Error(), err)
}
