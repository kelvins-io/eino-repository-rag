package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID     = "auth_user_id"
	ContextTenantID   = "auth_tenant_id"
	ContextTenantCode = "auth_tenant_code"
	ContextUsername   = "auth_username"
)

// Middleware 校验 Authorization: Bearer <jwt>
func Middleware(tm *TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "缺少 Authorization，请先登录",
			})
			return
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "Authorization 格式错误，应为 Bearer <token>",
			})
			return
		}
		claims, err := tm.Parse(strings.TrimSpace(raw[len(prefix):]))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "登录已失效，请重新登录",
			})
			return
		}
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextTenantID, claims.TenantID)
		c.Set(ContextTenantCode, claims.TenantCode)
		c.Set(ContextUsername, claims.Username)
		c.Next()
	}
}

// UserIDFromContext 读取鉴权后的业务 user_id
func UserIDFromContext(c *gin.Context) string {
	v, _ := c.Get(ContextUserID)
	s, _ := v.(string)
	return s
}

// UsernameFromContext 读取用户名
func UsernameFromContext(c *gin.Context) string {
	v, _ := c.Get(ContextUsername)
	s, _ := v.(string)
	return s
}

// TenantCodeFromContext 读取租户 Code
func TenantCodeFromContext(c *gin.Context) string {
	v, _ := c.Get(ContextTenantCode)
	s, _ := v.(string)
	return s
}
