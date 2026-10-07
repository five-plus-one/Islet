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

// all: 前缀不可省略：默认 embed 会排除 _ 和 . 开头的文件，
// 而 Vite 产物中存在 _plugin-vue_export-helper-*.js 这类下划线开头的 chunk。
// /assets/ 下缺失的文件显式 404，不能回退 index.html（模块加载器会因 MIME 拒绝）。
//go:embed all:admin
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
		if strings.HasPrefix(uriPath, "/assets/") {
			response.NotFound(c, "not found")
			return
		}
		// SPA 前端路由回退
		serve(c, "index.html")
	})
}
