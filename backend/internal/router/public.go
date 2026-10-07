package router

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"islet/backend/internal/middleware"
	"islet/backend/internal/model"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
	"islet/backend/internal/storage"
)

// registerImageRoutes 图片访问：/i/{key} 原图，/t/{key} 缩略图（缺失回退原图）。
func registerImageRoutes(engine *gin.Engine, deps *Deps) {
	handler := func(thumb bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			serveImage(c, deps, c.Param("key"), thumb)
		}
	}
	for _, path := range []string{"/i/*key", "/t/*key"} {
		engine.GET(path, handler(path == "/t/*key"))
		engine.HEAD(path, handler(path == "/t/*key"))
	}
}

func serveImage(c *gin.Context, deps *Deps, rawKey string, thumb bool) {
	key := strings.TrimPrefix(rawKey, "/")
	if !storage.SafeKey(key) || deps.Images == nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	image, err := deps.Images.GetByKey(key)
	if err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	targetKey := key
	if thumb {
		if image.ThumbKey == nil || *image.ThumbKey == "" {
			thumb = false // 回退原图
		} else {
			targetKey = *image.ThumbKey
		}
	}
	driver, _, err := deps.Policies.Driver(image.PolicyID)
	if err != nil {
		c.String(http.StatusNotFound, "policy unavailable")
		return
	}

	etag := etagOf(targetKey)
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}

	switch driver.Access() {
	case storage.AccessRedirect:
		c.Redirect(http.StatusFound, driver.RedirectURL(targetKey))
	case storage.AccessLocal, storage.AccessProxy:
		content, size, err := driver.Read(c.Request.Context(), targetKey)
		if err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		defer content.Close()
		contentType := image.Mime
		if thumb {
			contentType = thumbContentType(image, targetKey)
		}
		c.DataFromReader(http.StatusOK, size, contentType, content, map[string]string{})
	default:
		c.String(http.StatusNotFound, "not found")
	}
}

func thumbContentType(image model.Image, key string) string {
	if strings.HasSuffix(key, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

func etagOf(key string) string {
	// key 唯一即内容唯一；截断避免超长头部
	trimmed := key
	if len(trimmed) > 24 {
		trimmed = trimmed[len(trimmed)-24:]
	}
	return `"` + strings.ReplaceAll(trimmed, `"`, "") + `"`
}

// registerPublicUpload 公网端口上传（Token / OIDC / 会话均可）。
func registerPublicUpload(api *gin.RouterGroup, deps *Deps) {
	api.POST("/upload", deps.Authenticator.Middleware(), func(c *gin.Context) {
		user, ok := middleware.CurrentUser(c)
		if !ok {
			response.Unauthorized(c, "unauthorized")
			return
		}
		if user.Status != model.StatusActive {
			response.Forbidden(c, "账号不可用")
			return
		}
		handleUpload(c, deps, user)
	})
}

// handleUpload 上传共享逻辑（Web 与 API 同链路）。
func handleUpload(c *gin.Context, deps *Deps, user middleware.AuthUser) {
	maxBytes := deps.Settings.GetInt64(service.SettingUploadMaxBytes, 100<<20)
	files, err := service.ParseUploadForm(c, maxBytes)
	if err != nil {
		uploadError(c, err)
		return
	}
	picgo := c.Query("picgo") == "1" || c.PostForm("picgo") == "1"
	policyID := parseUint64(c.PostForm("policy_id"))
	albumID := parseUint64(c.PostForm("album_id"))
	var albumPtr *uint64
	if albumID != 0 {
		albumPtr = &albumID
	}
	tags := parseTags(c.PostForm("tags"))

	items := make([]service.ImageItem, 0, len(files))
	urls := make([]string, 0, len(files))
	var firstErr error
	for _, header := range files {
		f, err := header.Open()
		if err != nil {
			firstErr = err
			break
		}
		var buf bytes.Buffer
		_, copyErr := io.Copy(&buf, f)
		_ = f.Close()
		if copyErr != nil {
			firstErr = copyErr
			break
		}
		result, err := deps.Uploads.Upload(c.Request.Context(), service.UploadInput{
			UserID:   user.ID,
			GroupID:  user.GroupID,
			IsSuper:  user.Role == model.UserRoleSuperAdmin,
			FileName: header.Filename,
			Content:  &buf,
			PolicyID: policyID,
			AlbumID:  albumPtr,
			Tags:     tags,
		})
		if err != nil {
			firstErr = err
			break
		}
		item := deps.Images.ItemView(result.Image)
		items = append(items, item)
		urls = append(urls, item.URL)
	}

	if picgo {
		if firstErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": firstErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "result": urls})
		return
	}
	if firstErr != nil {
		uploadError(c, firstErr)
		return
	}
	response.OK(c, gin.H{"items": items, "urls": urls})
}

func uploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUploadUnsupported),
		errors.Is(err, service.ErrUploadTooLarge),
		errors.Is(err, service.ErrUploadQuota),
		errors.Is(err, service.ErrUploadRateLimited),
		errors.Is(err, service.ErrUploadMimeBlocked),
		errors.Is(err, service.ErrGroupDisabled):
		response.Error(c, http.StatusBadRequest, 4500, err.Error())
	default:
		response.BadRequest(c, err.Error())
	}
}

func parseUint64(s string) uint64 {
	var n uint64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + uint64(r-'0')
	}
	return n
}

func parseTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}
