package logger

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinLogger 用 zap 记录 HTTP 访问日志。
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		skip := path == "/health"

		c.Next()

		if skip {
			return
		}

		latency := time.Since(start)
		status := c.Writer.Status()
		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Duration("latency", latency),
			zap.Int("body_size", c.Writer.Size()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		switch {
		case status >= http.StatusInternalServerError:
			L().Error("http request", fields...)
		case status >= http.StatusBadRequest:
			L().Warn("http request", fields...)
		default:
			L().Info("http request", fields...)
		}
	}
}

// GinRecovery 用 zap 记录 panic 并返回 500。
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					var se *os.SyscallError
					if errors.As(ne.Err, &se) {
						msg := strings.ToLower(se.Error())
						brokenPipe = strings.Contains(msg, "broken pipe") ||
							strings.Contains(msg, "connection reset by peer")
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				if brokenPipe {
					L().Error("broken pipe",
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
					)
					_ = c.Error(err.(error))
					c.Abort()
					return
				}

				fields := []zap.Field{
					zap.Any("error", err),
					zap.String("request", string(httpRequest)),
				}
				if stack {
					fields = append(fields, zap.ByteString("stack", debug.Stack()))
				}
				L().Error("panic recovered", fields...)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
