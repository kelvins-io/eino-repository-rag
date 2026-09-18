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
	case errors.Is(err, service.ErrNotPlatformAdmin):
		code = http.StatusForbidden
	case errors.Is(err, service.ErrUsernameTaken), errors.Is(err, service.ErrTenantCodeTaken):
		code = http.StatusConflict
	case strings.Contains(err.Error(), "不能为空"),
		strings.Contains(err.Error(), "至少"),
		strings.Contains(err.Error(), "过长"):
		code = http.StatusBadRequest
	}
	writeFail(c, code, err.Error(), err)
}
