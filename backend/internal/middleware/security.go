package middleware

import (
	"github.com/gin-gonic/gin"
	"islet/backend/internal/response"
	"net/http"
	"net/url"
	"strings"
)

// RequestProtection rejects cross-origin mutations and keeps account responses private.
func RequestProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("X-Frame-Options", "DENY")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
			if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
				if origin := c.GetHeader("Origin"); origin != "" {
					u, err := url.Parse(origin)
					if err != nil || !strings.EqualFold(u.Host, c.Request.Host) {
						response.Forbidden(c, "请求来源不受信任，请从本站重新操作")
						c.Abort()
						return
					}
				}
				if !strings.HasSuffix(c.Request.URL.Path, "/upload") {
					c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
				}
			}
		}
		c.Next()
	}
}
