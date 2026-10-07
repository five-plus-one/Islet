package router

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"islet/backend/internal/middleware"
	"islet/backend/internal/model"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

// registerMeRoutes 个人资料与配额、API Token 管理。
func registerMeRoutes(group *gin.RouterGroup, deps *Deps) {
	group.GET("/me", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		quota, used := meQuota(deps, user)
		groupInfo := map[string]any{}
		if g, err := deps.Groups.Get(user.GroupID); err == nil {
			groupInfo = map[string]any{
				"id": g.ID, "name": g.Name, "quota_bytes": g.QuotaBytes,
				"max_file_bytes": g.MaxFileBytes, "allowed_mimes": g.AllowedMimes,
				"policy_ids": g.PolicyIDs, "uploads_per_hour": g.UploadsPerHour,
			}
		}
		availablePolicies, _ := deps.Policies.List()
		response.OK(c, gin.H{
			"user": user, "group": groupInfo,
			"quota": gin.H{"limit": quota, "used": used},
			"policies": availablePolicies,
		})
	})

	group.GET("/me/tokens", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		views, err := deps.Tokens.List(user.ID)
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		response.OK(c, gin.H{"items": views})
	})

	group.POST("/me/tokens", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in struct {
			Name      string     `json:"name"`
			ExpiresAt *time.Time `json:"expires_at"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.BadRequest(c, "请填写 Token 名称")
			return
		}
		if len([]rune(in.Name)) > 64 {
			response.BadRequest(c, "名称最多 64 字")
			return
		}
		plaintext, view, err := deps.Tokens.Create(user.ID, strings.TrimSpace(in.Name), in.ExpiresAt)
		if err != nil {
			response.ServerError(c, "创建失败")
			return
		}
		deps.Audit.Record(&user.ID, "token.create", "api_token", strconv.FormatUint(view.ID, 10), nil, clientIP(c))
		response.OK(c, gin.H{"token": plaintext, "item": view})
	})

	group.DELETE("/me/tokens/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Tokens.Revoke(user.ID, id); err != nil {
			response.NotFound(c, "Token 不存在")
			return
		}
		response.OK(c, gin.H{"revoked": true})
	})
}

func meQuota(deps *Deps, user middleware.AuthUser) (limit uint64, used uint64) {
	if user.Role == model.UserRoleSuperAdmin {
		return 0, 0 // 不限
	}
	if g, err := deps.Groups.Get(user.GroupID); err == nil {
		limit = g.QuotaBytes
	}
	if used, err := deps.Uploads.UsedBytes(user.ID); err == nil {
		return limit, used
	}
	return limit, 0
}

// registerImageManageRoutes 用户侧图库 / 回收站。
func registerImageManageRoutes(group *gin.RouterGroup, deps *Deps) {
	group.GET("/images", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		q := service.ImageQuery{
			Tag:     c.Query("tag"),
			Mime:    c.Query("mime"),
			Keyword: c.Query("keyword"),
			Recycled: c.Query("recycled") == "1",
		}
		if q.Recycled && user.Role == model.UserRoleUser {
			response.Forbidden(c, "回收站总览仅管理员可见")
			return
		}
		if user.Role != model.UserRoleUser {
			if uid := parseUint64(c.Query("user_id")); uid != 0 {
				q.UserID = uid
			}
		}
		if albumID := parseUint64(c.Query("album_id")); albumID != 0 {
			q.AlbumID = &albumID
		}
		if before := c.Query("before"); before != "" {
			if t, err := time.Parse(time.RFC3339, before); err == nil {
				q.Before = &t
			}
		}
		if limit := parseUint64(c.Query("limit")); limit > 0 && limit <= 100 {
			q.Limit = int(limit)
		}
		items, err := deps.Images.List(q)
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		response.OK(c, gin.H{"items": items})
	})

	group.PATCH("/images/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var in service.ImageUpdate
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		image, err := deps.Images.Update(id, user.ID, user.Role == model.UserRoleSuperAdmin, in)
		if err != nil {
			manageImageError(c, err)
			return
		}
		response.OK(c, deps.Images.ItemView(image))
	})

	group.DELETE("/images/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Images.Recycle(id, user.ID, user.Role == model.UserRoleSuperAdmin); err != nil {
			manageImageError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "image.recycle", "image", c.Param("id"), nil, clientIP(c))
		response.OK(c, gin.H{"recycled": true})
	})

	group.POST("/images/:id/restore", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		err := deps.Images.Restore(id, user.ID, user.Role == model.UserRoleSuperAdmin, func(owner uint64, add uint64) error {
			if user.Role == model.UserRoleSuperAdmin && owner != user.ID {
				return nil
			}
			if g, err := deps.Groups.Get(user.GroupID); err == nil && g.QuotaBytes > 0 {
				used, err := deps.Uploads.UsedBytes(owner)
				if err != nil {
					return err
				}
				if used+add > g.QuotaBytes {
					return service.ErrUploadQuota
				}
			}
			return nil
		})
		if err != nil {
			manageImageError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "image.restore", "image", c.Param("id"), nil, clientIP(c))
		response.OK(c, gin.H{"restored": true})
	})

	group.DELETE("/images/:id/purge", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Images.Purge(c.Request.Context(), id, user.ID, user.Role == model.UserRoleSuperAdmin); err != nil {
			manageImageError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "image.purge", "image", c.Param("id"), nil, clientIP(c))
		response.OK(c, gin.H{"purged": true})
	})
}

func manageImageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrImageNotFound), errors.Is(err, service.ErrAlbumNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrUploadQuota):
		response.Error(c, http.StatusBadRequest, 4500, err.Error())
	default:
		response.BadRequest(c, err.Error())
	}
}

// registerAlbumRoutes 相册 CRUD。
func registerAlbumRoutes(group *gin.RouterGroup, deps *Deps) {
	group.GET("/albums", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		albums, err := deps.Albums.List(user.ID)
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		items := make([]gin.H, 0, len(albums))
		for _, album := range albums {
			var count int64
			_ = deps.DB.Model(&model.Image{}).Where("album_id = ? AND deleted_at IS NULL", album.ID).Count(&count).Error
			items = append(items, gin.H{
				"id": album.ID, "name": album.Name, "description": album.Description,
				"image_count": count, "created_at": album.CreatedAt,
			})
		}
		response.OK(c, gin.H{"items": items})
	})
	group.POST("/albums", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in service.AlbumInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		album, err := deps.Albums.Create(user.ID, in)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.OK(c, album)
	})
	group.PATCH("/albums/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var in service.AlbumInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		album, err := deps.Albums.Update(user.ID, id, in)
		if err != nil {
			response.NotFound(c, err.Error())
			return
		}
		response.OK(c, album)
	})
	group.DELETE("/albums/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Albums.Delete(user.ID, id); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.OK(c, gin.H{"deleted": true})
	})
}

func clientIP(c *gin.Context) string { return c.ClientIP() }
