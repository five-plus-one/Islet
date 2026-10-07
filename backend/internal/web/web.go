package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"

	"islet/backend/internal/response"
)

//go:embed admin
var adminFS embed.FS

// RegisterAdminSPA 把嵌入的 Vue SPA 挂到管理端口（NoRoute 兜底，API 404 保持 JSON）。
// 不走 http.FileServer：其对 /index.html 的 301 规则会让根路径陷入重定向循环。
func RegisterAdminSPA(engine *gin.Engine) {
	sub, err := fs.Sub(adminFS, "admin")
	if err != nil {
		return
	}
	serve := func(c *gin.Context, name string) {
		data, err := fs.ReadFile(sub, name)
		if err != nil {
			response.NotFound(c, "not found")
			return
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Data(http.StatusOK, contentType, data)
	}
	engine.NoRoute(func(c *gin.Context) {
		uriPath := c.Request.URL.Path
		if strings.HasPrefix(uriPath, "/api/") {
			response.NotFound(c, "not found")
			return
		}
		name := strings.TrimPrefix(uriPath, "/")
		if name == "" || strings.HasSuffix(uriPath, "/") {
			serve(c, "index.html")
			return
		}
		if fs.ValidPath(name) {
			if f, err := sub.Open(name); err == nil {
				_ = f.Close()
				serve(c, name)
				return
			}
		}
		// SPA 前端路由回退
		serve(c, "index.html")
	})
}
