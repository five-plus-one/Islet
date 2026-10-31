package router

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"islet/backend/internal/middleware"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

func registerModerationRoutes(group *gin.RouterGroup, deps *Deps) {
	group.PATCH("/images/:id/moderation", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			response.BadRequest(c, "图片编号无效")
			return
		}
		var in struct {
			Blocked *bool  `json:"blocked"`
			Reason  string `json:"reason"`
		}
		if c.ShouldBindJSON(&in) != nil || in.Blocked == nil {
			response.BadRequest(c, "请选择图片访问状态")
			return
		}
		user, _ := middleware.CurrentUser(c)
		image, err := deps.Images.Moderate(id, user.ID, *in.Blocked, in.Reason, clientIP(c))
		if err != nil {
			switch {
			case errors.Is(err, service.ErrImageNotFound):
				response.NotFound(c, "图片不存在")
			case errors.Is(err, service.ErrModerationReason):
				response.BadRequest(c, err.Error())
			default:
				response.ServerError(c, "保存审核结果失败，请稍后重试")
			}
			return
		}
		response.OK(c, deps.Images.ItemView(image))
	})
	preview := func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			response.BadRequest(c, "图片编号无效")
			return
		}
		image, err := deps.Images.Get(id)
		if err != nil || image.DeletedAt.Valid {
			if err != nil && !errors.Is(err, service.ErrImageNotFound) {
				response.ServerError(c, "读取图片失败")
				return
			}
			response.NotFound(c, "图片不存在")
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
		serveStoredImage(c, deps, image, c.Query("thumb") == "1")
	}
	group.GET("/images/:id/preview", preview)
	group.HEAD("/images/:id/preview", preview)
}

// A successful image response lets existing <img> embeds display the policy notice.
func servePolicyNotice(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", policyNotice)
}
