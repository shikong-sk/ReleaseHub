package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// JWT 认证中间件

var jwtSecret = []byte("releasehub-default-secret-change-me")

// SetJWTSecret 设置 JWT 签名密钥
func SetJWTSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
	}
}

type Claims struct {
	UserID   uint   `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 生成 JWT Token
func GenerateToken(user models.User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "releasehub",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// AuthRequired 验证 JWT Token 的中间件
func AuthRequired(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization 头获取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 也检查 cookie
			if cookie, err := c.Cookie("releasehub_token"); err == nil && cookie != "" {
				authHeader = "Bearer " + cookie
			}
		}

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证凭证"})
			c.Abort()
			return
		}

		// 解析 Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		var tokenString string
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			tokenString = parts[1]
		} else {
			tokenString = authHeader
		}

		// 解析并验证 token
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("不支持的签名方法: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token 无效或已过期"})
			c.Abort()
			return
		}

		// 检查用户是否仍然有效
		var user models.User
		if err := db.WithContext(c.Request.Context()).First(&user, claims.UserID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
			c.Abort()
			return
		}
		if !user.Enabled {
			c.JSON(http.StatusForbidden, gin.H{"error": "用户已被禁用"})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("userID", user.ID)
		c.Set("username", user.Username)
		c.Set("role", user.Role)
		c.Set("authType", "jwt")
		c.Next()
	}
}

// AdminRequired 要求管理员角色
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role.(string) != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}
