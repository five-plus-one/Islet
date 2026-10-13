package router

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"islet/backend/internal/middleware"
	"islet/backend/internal/model"
	"islet/backend/internal/response"
	"islet/backend/internal/service"
)

// registerPolicyRoutes 存储策略管理（admin+）。
func registerPolicyRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.GET("/storage-policies", func(c *gin.Context) {
		views, err := deps.Policies.List()
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		response.OK(c, gin.H{"items": views})
	})

	admin.POST("/storage-policies", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in service.PolicyInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		view, err := deps.Policies.Create(in)
		if err != nil {
			policyError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "policy.create", "storage_policy", strconv.FormatUint(view.ID, 10), nil, clientIP(c))
		response.OK(c, view)
	})

	admin.PATCH("/storage-policies/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var in service.PolicyInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		in.ID = id
		view, err := deps.Policies.Update(id, in)
		if err != nil {
			policyError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "policy.update", "storage_policy", c.Param("id"), nil, clientIP(c))
		response.OK(c, view)
	})

	admin.DELETE("/storage-policies/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Policies.Delete(id); err != nil {
			policyError(c, err)
			return
		}
		deps.Audit.Record(&user.ID, "policy.delete", "storage_policy", c.Param("id"), nil, clientIP(c))
		response.OK(c, gin.H{"deleted": true})
	})

	// 连通测试：已存策略 POST /:id/test，草稿 POST /test
	admin.POST("/storage-policies/test", func(c *gin.Context) {
		var in service.PolicyInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		if err := deps.Policies.Test(c.Request.Context(), in); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.OK(c, gin.H{"ok": true, "message": "连接正常，测试对象已写入并清理"})
	})
	admin.POST("/storage-policies/:id/test", func(c *gin.Context) {
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var policy model.StoragePolicy
		if err := deps.DB.First(&policy, id).Error; err != nil {
			response.NotFound(c, "存储策略不存在")
			return
		}
		driver, _, err := deps.Policies.Driver(id)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		if err := driver.Check(c.Request.Context()); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.OK(c, gin.H{"ok": true, "message": "连接正常"})
	})
}

func policyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPolicyNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrSecretUndecryptable):
		response.Error(c, http.StatusBadRequest, 4003, err.Error())
	case errors.Is(err, service.ErrPolicyNameExists),
		errors.Is(err, service.ErrPolicyInUse):
		response.Error(c, http.StatusConflict, 4009, err.Error())
	default:
		response.BadRequest(c, err.Error())
	}
}

// registerGroupRoutes 用户分组管理（admin+）。
func registerGroupRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.GET("/user-groups", func(c *gin.Context) {
		groups, err := deps.Groups.List()
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		items := make([]gin.H, 0, len(groups))
		for _, g := range groups {
			var members int64
			_ = deps.DB.Model(&model.User{}).Where("group_id = ? AND deleted_at IS NULL", g.ID).Count(&members).Error
			items = append(items, gin.H{
				"id": g.ID, "name": g.Name, "description": g.Description,
				"quota_bytes": g.QuotaBytes, "max_file_bytes": g.MaxFileBytes,
				"allowed_mimes": g.AllowedMimes, "policy_ids": g.PolicyIDs,
				"uploads_per_hour": g.UploadsPerHour, "built_in": g.BuiltIn,
				"status": g.Status, "members": members,
			})
		}
		response.OK(c, gin.H{"items": items})
	})
	admin.POST("/user-groups", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in service.GroupInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		group, err := deps.Groups.Create(in)
		if err != nil {
			response.Error(c, http.StatusConflict, 4009, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "group.create", "user_group", strconv.FormatUint(group.ID, 10), nil, clientIP(c))
		response.OK(c, group)
	})
	admin.PATCH("/user-groups/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var in service.GroupInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		group, err := deps.Groups.Update(id, in)
		if err != nil {
			response.Error(c, http.StatusConflict, 4009, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "group.update", "user_group", c.Param("id"), nil, clientIP(c))
		response.OK(c, group)
	})
	admin.DELETE("/user-groups/:id", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		if err := deps.Groups.Delete(id); err != nil {
			response.Error(c, http.StatusConflict, 4009, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "group.delete", "user_group", c.Param("id"), nil, clientIP(c))
		response.OK(c, gin.H{"deleted": true})
	})
}

// registerUserRoutes 用户管理（admin+；角色调整仅 super_admin）。
func registerUserRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.GET("/users", func(c *gin.Context) {
		keyword := strings.TrimSpace(c.Query("keyword"))
		status := c.Query("status")
		tx := deps.DB.Model(&model.User{})
		if keyword != "" {
			like := "%" + keyword + "%"
			tx = tx.Where("username LIKE ? OR email LIKE ?", like, like)
		}
		if status == model.StatusPending || status == model.StatusActive || status == model.StatusDisabled {
			tx = tx.Where("status = ?", status)
		}
		var users []model.User
		if err := tx.Order("id ASC").Limit(500).Find(&users).Error; err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		groups, _ := deps.Groups.List()
		groupNames := map[uint64]string{}
		for _, g := range groups {
			groupNames[g.ID] = g.Name
		}
		items := make([]gin.H, 0, len(users))
		for _, u := range users {
			email := ""
			if u.Email != nil {
				email = *u.Email
			}
			items = append(items, gin.H{
				"id": u.ID, "username": u.Username, "email": email,
				"auth_source": u.AuthSource, "role": u.Role, "status": u.Status,
				"group_id": u.GroupID, "group_name": groupNames[u.GroupID],
				"last_login_at": u.LastLoginAt, "created_at": u.CreatedAt,
			})
		}
		response.OK(c, gin.H{"items": items})
	})

	admin.PATCH("/users/:id/group", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
		var in struct {
			GroupID uint64 `json:"group_id"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		if _, err := deps.Groups.Get(in.GroupID); err != nil {
			response.BadRequest(c, "目标分组不存在")
			return
		}
		if err := deps.DB.Model(&model.User{}).Where("id = ? AND role <> ?", id, model.UserRoleSuperAdmin).
			Update("group_id", in.GroupID).Error; err != nil {
			response.ServerError(c, "更新失败")
			return
		}
		deps.Audit.Record(&user.ID, "user.group", "user", c.Param("id"), gin.H{"group_id": in.GroupID}, clientIP(c))
		response.OK(c, gin.H{"updated": true})
	})

	super := admin.Group("")
	super.Use(middleware.RequireSuperAdmin())
	super.POST("/users", func(c *gin.Context) {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
			GroupID  uint64 `json:"group_id"`
		}
		if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Username) == "" || len([]rune(in.Username)) > 64 {
			response.BadRequest(c, "请填写 1–64 字的用户名")
			return
		}
		if _, err := deps.Groups.Get(in.GroupID); err != nil {
			response.BadRequest(c, "请选择有效的用户分组")
			return
		}
		hash, err := service.HashPassword(in.Password)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		var count int64
		if err := deps.DB.Model(&model.User{}).Where("auth_source = ? AND username = ?", model.AuthSourceLocal, strings.TrimSpace(in.Username)).Count(&count).Error; err != nil {
			response.ServerError(c, "无法检查用户名")
			return
		}
		if count > 0 {
			response.BadRequest(c, "用户名已存在")
			return
		}
		user := model.User{Username: strings.TrimSpace(in.Username), PasswordHash: &hash, AuthSource: model.AuthSourceLocal, Role: model.UserRoleUser, Status: model.StatusActive, GroupID: in.GroupID}
		if err := deps.DB.Create(&user).Error; err != nil {
			response.ServerError(c, "创建用户失败")
			return
		}
		response.OK(c, gin.H{"id": user.ID, "username": user.Username})
	})
	super.PATCH("/users/:id/role", func(c *gin.Context) {
		var in struct {
			Role string `json:"role"`
		}
		if err := c.ShouldBindJSON(&in); err != nil ||
			(in.Role != model.UserRoleAdmin && in.Role != model.UserRoleUser) {
			response.BadRequest(c, "role 须为 admin 或 user")
			return
		}
		if err := deps.DB.Model(&model.User{}).Where("id = ? AND role <> ?", c.Param("id"), model.UserRoleSuperAdmin).
			Update("role", in.Role).Error; err != nil {
			response.ServerError(c, "更新失败")
			return
		}
		deps.Audit.Record(nil, "user.role", "user", c.Param("id"), gin.H{"role": in.Role}, clientIP(c))
		response.OK(c, gin.H{"updated": true})
	})
	super.PATCH("/users/:id/status", func(c *gin.Context) {
		var in struct {
			Status string `json:"status"`
		}
		if err := c.ShouldBindJSON(&in); err != nil ||
			(in.Status != model.StatusActive && in.Status != model.StatusDisabled && in.Status != model.StatusPending) {
			response.BadRequest(c, "status 不合法")
			return
		}
		err := deps.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.User{}).Where("id = ? AND role <> ?", c.Param("id"), model.UserRoleSuperAdmin).
				Update("status", in.Status).Error; err != nil {
				return err
			}
			if in.Status == model.StatusDisabled {
				var id uint64
				id, _ = strconv.ParseUint(c.Param("id"), 10, 64)
				if err := tx.Where("user_id = ?", id).Delete(&model.AuthSession{}).Error; err != nil {
					return err
				}
				return tx.Model(&model.APIToken{}).Where("user_id = ?", id).Update("status", model.StatusDisabled).Error
			}
			return nil
		})
		if err != nil {
			response.ServerError(c, "更新失败")
			return
		}
		deps.Audit.Record(nil, "user.status", "user", c.Param("id"), gin.H{"status": in.Status}, clientIP(c))
		response.OK(c, gin.H{"updated": true})
	})
}

// registerStatsRoutes 统计看板（admin+）。
func registerStatsRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.GET("/stats/overview", func(c *gin.Context) {
		overview, err := deps.Stats.Overview()
		if err != nil {
			response.ServerError(c, "统计查询失败")
			return
		}
		response.OK(c, overview)
	})
	admin.GET("/stats/trend", func(c *gin.Context) {
		days := 14
		if v := parseUint64(c.Query("days")); v > 0 && v <= 90 {
			days = int(v)
		}
		trend, err := deps.Stats.Trend(days)
		if err != nil {
			response.ServerError(c, "统计查询失败")
			return
		}
		response.OK(c, gin.H{"items": trend})
	})
	admin.GET("/stats/groups", func(c *gin.Context) {
		usage, err := deps.Stats.GroupUsage()
		if err != nil {
			response.ServerError(c, "统计查询失败")
			return
		}
		response.OK(c, gin.H{"items": usage})
	})
}

// registerSettingsRoutes 站点设置（admin+）。
func registerSettingsRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.POST("/auth/test-oidc", func(c *gin.Context) {
		var in struct {
			Issuer string `json:"issuer"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Issuer) == "" {
			response.BadRequest(c, "请填写 Issuer")
			return
		}
		meta, err := deps.OIDC.Discover(c.Request.Context(), in.Issuer)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.OK(c, gin.H{
			"issuer":                 meta.Issuer,
			"authorization_endpoint": meta.AuthorizationEndpoint,
			"token_endpoint":         meta.TokenEndpoint,
			"jwks_uri":               meta.JWKSURI,
		})
	})
	admin.GET("/settings", func(c *gin.Context) {
		values, err := deps.Settings.All()
		if err != nil {
			response.ServerError(c, "读取失败")
			return
		}
		response.OK(c, values)
	})
	admin.PUT("/settings", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in map[string]string
		if err := c.ShouldBindJSON(&in); err != nil {
			response.BadRequest(c, "参数错误")
			return
		}
		local := deps.Settings.Get(service.SettingAuthLocalEnabled)
		if v, ok := in[service.SettingAuthLocalEnabled]; ok {
			local = v
		}
		issuer := deps.Settings.Get(service.SettingAuthOIDCIssuer)
		client := deps.Settings.Get(service.SettingAuthOIDCClientID)
		if v, ok := in[service.SettingAuthOIDCIssuer]; ok {
			issuer = v
		}
		if v, ok := in[service.SettingAuthOIDCClientID]; ok {
			client = v
		}
		if issuer == "" {
			issuer = deps.Cfg.OIDCIssuer
		}
		if client == "" {
			client = deps.Cfg.OIDCClientID
		}
		if local == "false" && (strings.TrimSpace(issuer) == "" || strings.TrimSpace(client) == "") {
			response.BadRequest(c, "请先配置统一登录，再关闭本地登录")
			return
		}
		if err := deps.Settings.Put(in); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "settings.update", "settings", "", in, clientIP(c))
		values, _ := deps.Settings.All()
		response.OK(c, values)
	})
}

// registerSystemRoutes 回收站总览 / 清空 / 恢复初始化（super_admin）。
func registerSystemRoutes(admin *gin.RouterGroup, deps *Deps) {
	admin.GET("/recycle", func(c *gin.Context) {
		items, err := deps.Images.RecycleList(100)
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		response.OK(c, gin.H{"items": items})
	})
	admin.POST("/recycle/purge-all", middleware.RequireSuperAdmin(), func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		count, err := deps.Images.PurgeAllRecycle(c.Request.Context())
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "recycle.purge_all", "system", "", gin.H{"count": count}, clientIP(c))
		response.OK(c, gin.H{"purged": count})
	})

	super := admin.Group("")
	super.Use(middleware.RequireSuperAdmin())
	super.GET("/audit-logs", func(c *gin.Context) {
		var before *time.Time
		if v := c.Query("before"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				before = &t
			}
		}
		items, err := deps.Audit.List(before, 50)
		if err != nil {
			response.ServerError(c, "查询失败")
			return
		}
		response.OK(c, gin.H{"items": items})
	})
	super.POST("/system/reset", func(c *gin.Context) {
		user, _ := middleware.CurrentUser(c)
		var in struct {
			Confirm  string `json:"confirm"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&in); err != nil || in.Confirm != "RESET" {
			response.BadRequest(c, "请输入确认短语 RESET")
			return
		}
		// 本地来源的 super_admin 需验证密码；OIDC 来源仅确认短语
		var current model.User
		if err := deps.DB.First(&current, user.ID).Error; err == nil &&
			current.AuthSource == model.AuthSourceLocal && current.PasswordHash != nil {
			if err := deps.Auth.VerifyLocalPassword(user.ID, in.Password); err != nil {
				response.Forbidden(c, "密码验证失败")
				return
			}
		}
		if resetSystemFn == nil {
			response.ServerError(c, "reset unavailable")
			return
		}
		if err := resetSystemFn(); err != nil {
			response.ServerError(c, err.Error())
			return
		}
		deps.Audit.Record(&user.ID, "system.reset", "system", "", nil, clientIP(c))
		response.OK(c, gin.H{"reset": true})
	})
}

// resetSystemFn 由 app 注入（app.ResetSetup）。
var resetSystemFn func() error

// SetResetSystem 供 app 注入。
func SetResetSystem(fn func() error) { resetSystemFn = fn }
