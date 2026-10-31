package router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

func registerClientConfigRoutes(group *gin.RouterGroup, deps *Deps) {
	group.GET("/client-configs/piclist", func(c *gin.Context) {
		base := strings.TrimRight(deps.Settings.Get(service.SettingPublicBaseURL), "/")
		if base == "" {
			base = strings.TrimRight(deps.Cfg.AdminBaseURL, "/")
		}
		response.OK(c, gin.H{
			"uploader":         "advancedplist",
			"endpoint":         base + "/api/v1/upload",
			"method":           "POST",
			"formDataKey":      "files",
			"headers":          `{"Authorization":"Bearer YOUR_ISLET_UPLOAD_TOKEN"}`,
			"body":             "{}",
			"resDataPath":      "data.items.0.url",
			"uploadScriptName": "", "customPrefix": "", "webPath": "",
		})
	})
}
