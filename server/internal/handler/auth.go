package handler

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"wendaoiotpanel/internal/model"
	"wendaoiotpanel/internal/store"
	cryptopkg "wendaoiotpanel/pkg/crypto"
	"wendaoiotpanel/pkg/token"

	"github.com/gin-gonic/gin"
)

// tokenMgr 由 main 通过 InitJWTSecret 初始化。
var tokenMgr *token.Manager

// tokenTTL 控制登录 token 有效期。
var tokenTTL = 24 * time.Hour

func InitJWTSecret(secret string) {
	tokenMgr = token.NewManager(secret, tokenTTL)
}

// Login 登录：角色以数据库为准，忽略前端传入的 role。
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role"` // 仅前端分流用，不作为授权依据
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		authFail(c)
		return
	}

	user, err := h.store.GetAdminUserByUsername(req.Username)
	if err != nil {
		authFail(c)
		return
	}
	if !cryptopkg.CheckPassword(user.Password, req.Password) {
		// 兼容历史裸 SHA-256：验证通过后自动升级为 bcrypt（无感迁移）。
		if !(cryptopkg.IsLegacySHA256(user.Password) &&
			cryptopkg.CheckLegacySHA256(user.Password, req.Password)) {
			authFail(c)
			return
		}
		if newHash, err := cryptopkg.HashPassword(req.Password); err == nil {
			if err := h.store.UpdateAdminUserPassword(user.ID, newHash); err != nil {
				log.Printf("upgrade legacy password hash for %s failed: %v", user.Username, err)
			}
			user.Password = newHash
		}
	}

	tokenStr, err := tokenMgr.Issue(token.Claims{
		UserID:       user.ID,
		Username:     user.Username,
		Role:         user.Role,
		TenantID:     user.TenantID,
		TokenVersion: user.TokenVersion,
	})
	if err != nil {
		authFail(c)
		return
	}

	success(c, gin.H{
		"token": tokenStr,
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"role":      user.Role,
			"tenant_id": user.TenantID,
		},
	})
}

func authFail(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "用户名或密码错误"})
}

// AuthMiddleware 校验 JWT：解析后每次查库，确认用户仍存在且 token_version 未变更。
// 用户被删除、改密、重置密码后，旧 token 立即失效。
func AuthMiddleware(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "unauthorized"})
			return
		}
		claims, err := tokenMgr.Parse(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "invalid token"})
			return
		}

		user, err := s.GetAdminUserByIDFull(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "用户不存在或已停用"})
			return
		}
		if user.TokenVersion != claims.TokenVersion {
			c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "登录已失效，请重新登录"})
			return
		}

		// 以库中最新身份为准，不信任 token 内的角色快照
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Set("tenant_id", user.TenantID)
		c.Next()
	}
}

// RequireRole 限制仅指定角色可访问。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		role := c.GetString("role")
		if !allowed[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, Response{Code: 403, Msg: "无权操作"})
			return
		}
		c.Next()
	}
}

// SeedAdminUsers 启动时确保超级管理员存在。
// 优先级：环境变量 WQ_ADMIN_PASSWORD；未提供则生成随机强密码并仅在启动日志打印一次。
func SeedAdminUsers(s *store.Store) {
	username := os.Getenv("WQ_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	if _, err := s.GetAdminUserByUsername(username); err == nil {
		return // 已存在，不改密
	}

	pwd := os.Getenv("WQ_ADMIN_PASSWORD")
	if pwd == "" {
		generated, err := cryptopkg.RandomPassword(16)
		if err != nil {
			log.Printf("seed admin: 生成随机密码失败: %v", err)
			return
		}
		pwd = generated
		log.Printf("======================================================")
		log.Printf("首次启动：已创建超级管理员 %s，初始密码（仅显示一次，请立即登录修改）: %s", username, pwd)
		log.Printf("也可通过环境变量 WQ_ADMIN_PASSWORD 指定，或设置 WQ_ADMIN_USERNAME 改用户名")
		log.Printf("======================================================")
	}

	hash, err := cryptopkg.HashPassword(pwd)
	if err != nil {
		log.Printf("seed admin: hash error: %v", err)
		return
	}
	if err := s.CreateAdminUser(&model.AdminUser{
		Username: username,
		Password: hash,
		Role:     model.RoleSuperAdmin,
	}); err != nil {
		log.Printf("seed admin: create error: %v", err)
	}
}

// ===== 用户管理 =====

func (h *Handler) ListUsers(c *gin.Context) {
	role, tenantID := getAuthInfo(c)
	if role != model.RoleSuperAdmin {
		tenantID = getTenantID(c)
	}
	users, err := h.store.ListAdminUsers(tenantID)
	if err != nil {
		fail(c, -1, err.Error())
		return
	}
	success(c, users)
}

func (h *Handler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, Response{Code: 401, Msg: "unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	uid := userID.(uint)
	persisted, err := h.store.GetAdminUserByIDFull(uid)
	if err != nil {
		fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	fullUser, err := h.store.GetAdminUserByUsername(persisted.Username)
	if err != nil {
		fail(c, http.StatusNotFound, "用户不存在")
		return
	}
	if !cryptopkg.CheckPassword(fullUser.Password, req.OldPassword) {
		fail(c, http.StatusBadRequest, "原密码不正确")
		return
	}
	newHash, err := cryptopkg.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.UpdateAdminUserPasswordAndBump(uid, newHash); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

// canManageUser 判定当前操作者能否管理目标用户（用于重置密码/删除）。
func (h *Handler) canManageUser(c *gin.Context, targetID uint) bool {
	role, tenantID := getAuthInfo(c)
	target, err := h.store.GetAdminUserByIDFull(targetID)
	if err != nil {
		return false
	}
	if role == model.RoleSuperAdmin {
		return true
	}
	// 租户管理员：只能管理本租户的租户管理员，不能动超管。
	if target.Role == model.RoleSuperAdmin {
		return false
	}
	if tenantID == nil || target.TenantID == nil || *target.TenantID != *tenantID {
		return false
	}
	return true
}

func (h *Handler) AdminResetUserPassword(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if !h.canManageUser(c, uint(id)) {
		c.AbortWithStatusJSON(http.StatusForbidden, Response{Code: 403, Msg: "无权操作此用户"})
		return
	}
	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	newHash, err := cryptopkg.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.UpdateAdminUserPasswordAndBump(uint(id), newHash); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	userID, _ := c.Get("user_id")
	if uint(id) == userID.(uint) {
		fail(c, http.StatusBadRequest, "不能删除自己")
		return
	}
	if !h.canManageUser(c, uint(id)) {
		c.AbortWithStatusJSON(http.StatusForbidden, Response{Code: 403, Msg: "无权操作此用户"})
		return
	}
	if err := h.store.DeleteAdminUser(uint(id)); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	success(c, nil)
}

func getTenantID(c *gin.Context) *uint {
	val, exists := c.Get("tenant_id")
	if !exists || val == nil {
		return nil
	}
	if tid, ok := val.(*uint); ok {
		return tid
	}
	return nil
}

func parseUintParam(c *gin.Context, param string) (uint64, error) {
	s := c.Param(param)
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
