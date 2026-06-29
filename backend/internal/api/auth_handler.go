package api

import (
	"errors"
	"net/http"
	"time"

	"releasehub/backend/internal/middleware"
	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type authHandler struct {
	db *gorm.DB
}

type loginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Token    string       `json:"token"`
	User     userResponse `json:"user"`
	ExpireAt string       `json:"expireAt"`
}

type userResponse struct {
	ID         uint    `json:"id"`
	Username   string  `json:"username"`
	Role       string  `json:"role"`
	Enabled    bool    `json:"enabled"`
	LastLoginAt *string `json:"lastLoginAt"`
	CreatedAt  string  `json:"createdAt"`
}

type createUserInput struct {
	Username string `json:"username" binding:"required,min=2,max=120"`
	Password string `json:"password" binding:"required,min=6,max=128"`
	Role     string `json:"role" binding:"required,oneof=admin operator viewer"`
}

type updatePasswordInput struct {
	OldPassword string `json:"oldPassword" binding:"required,min=1"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=128"`
}

func registerAuthRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &authHandler{db: db}

	router.POST("/api/auth/login", handler.login)
	router.GET("/api/auth/me", middleware.AuthRequired(db), handler.me)
	router.POST("/api/auth/change-password", middleware.AuthRequired(db), handler.changePassword)

	// 用户管理（仅管理员）
	users := router.Group("/api/users", middleware.AuthRequired(db), middleware.AdminRequired())
	users.GET("", handler.listUsers)
	users.POST("", handler.createUser)
	users.PATCH("/:id", handler.updateUser)
	users.DELETE("/:id", handler.deleteUser)
}

func (h *authHandler) login(c *gin.Context) {
	var input loginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	var user models.User
	if err := h.db.WithContext(c.Request.Context()).
		Where("username = ? AND enabled = ?", input.Username, true).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询用户失败")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		writeError(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token, err := middleware.GenerateToken(user)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "生成 Token 失败")
		return
	}

	// 更新最后登录时间
	now := time.Now().UTC()
	h.db.WithContext(c.Request.Context()).Model(&user).Update("last_login_at", now)

	expireAt := time.Now().Add(24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	c.JSON(http.StatusOK, loginResponse{
		Token:    token,
		User:     toUserResponse(user),
		ExpireAt: expireAt,
	})
}

func (h *authHandler) me(c *gin.Context) {
	userID := c.GetUint("userID")
	var user models.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, userID).Error; err != nil {
		writeError(c, http.StatusNotFound, "用户不存在")
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

func (h *authHandler) changePassword(c *gin.Context) {
	var input updatePasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	userID := c.GetUint("userID")
	var user models.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, userID).Error; err != nil {
		writeError(c, http.StatusNotFound, "用户不存在")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.OldPassword)); err != nil {
		writeError(c, http.StatusBadRequest, "当前密码错误")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "密码加密失败")
		return
	}

	h.db.WithContext(c.Request.Context()).Model(&user).Update("password_hash", string(hash))
	c.JSON(http.StatusOK, gin.H{"message": "密码已修改"})
}

func (h *authHandler) listUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&users).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询用户列表失败")
		return
	}
	items := make([]userResponse, 0, len(users))
	for _, u := range users {
		items = append(items, toUserResponse(u))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *authHandler) createUser(c *gin.Context) {
	var input createUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	// 检查用户名是否已存在
	var count int64
	h.db.WithContext(c.Request.Context()).Model(&models.User{}).Where("username = ?", input.Username).Count(&count)
	if count > 0 {
		writeError(c, http.StatusConflict, "用户名已存在")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "密码加密失败")
		return
	}

	user := models.User{
		Username:     input.Username,
		PasswordHash: string(hash),
		Role:         input.Role,
		Enabled:      true,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建用户失败")
		return
	}

	c.JSON(http.StatusCreated, toUserResponse(user))
}

func (h *authHandler) updateUser(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var user models.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "用户不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询用户失败")
		return
	}

	var input struct {
		Role    *string `json:"role"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	if input.Role != nil {
		user.Role = *input.Role
	}
	if input.Enabled != nil {
		user.Enabled = *input.Enabled
	}

	if err := h.db.WithContext(c.Request.Context()).Save(&user).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "更新用户失败")
		return
	}

	c.JSON(http.StatusOK, toUserResponse(user))
}

func (h *authHandler) deleteUser(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	// 不能删除自己
	if id == c.GetUint("userID") {
		writeError(c, http.StatusBadRequest, "不能删除当前登录用户")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&models.User{}, id).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除用户失败")
		return
	}

	c.Status(http.StatusNoContent)
}

func toUserResponse(u models.User) userResponse {
	var lastLogin *string
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.UTC().Format("2006-01-02T15:04:05Z")
		lastLogin = &s
	}
	return userResponse{
		ID:          u.ID,
		Username:    u.Username,
		Role:        u.Role,
		Enabled:     u.Enabled,
		LastLoginAt: lastLogin,
		CreatedAt:   u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
