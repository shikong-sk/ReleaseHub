package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ValidateAPIKey 验证请求中的 API Key
func ValidateAPIKey(db *gorm.DB, key string) (*models.APIKey, error) {
	if len(key) < 4 || key[:3] != "rh_" {
		return nil, fmt.Errorf("API Key 格式无效")
	}

	var apiKey models.APIKey
	if err := db.Where("key = ? AND enabled = ?", key, true).First(&apiKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("API Key 无效或已禁用")
		}
		return nil, err
	}

	// 更新最后使用时间
	now := time.Now().UTC()
	db.Model(&apiKey).Update("last_used_at", now)

	return &apiKey, nil
}

// APIKeyOrAuth 支持 API Key 或 JWT Token 认证
// 当 authEnabled 返回 false 时直接放行（认证关闭模式）
func APIKeyOrAuth(db *gorm.DB, authEnabled func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authEnabled() {
			c.Next()
			return
		}
		// 1. 尝试 API Key
		apiKeyHeader := c.GetHeader("X-API-Key")
		if apiKeyHeader == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer rh_") {
				apiKeyHeader = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if apiKeyHeader != "" && strings.HasPrefix(apiKeyHeader, "rh_") {
			key, err := ValidateAPIKey(db, apiKeyHeader)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				c.Abort()
				return
			}
			c.Set("apiKeyID", key.ID)
			c.Set("apiKeyScope", key.Scope)
			c.Set("authType", "apikey")
			c.Next()
			return
		}

		// 2. 尝试 JWT Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			if cookie, err := c.Cookie("releasehub_token"); err == nil && cookie != "" {
				authHeader = "Bearer " + cookie
			}
		}

		if authHeader != "" {
			AuthRequired(db)(c)
			return
		}

		c.Next()
	}
}

// GenerateAPIKey 生成随机 API Key
func GenerateAPIKey() (string, string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", err
	}
	key := "rh_" + hex.EncodeToString(keyBytes)
	hint := key[:8] + "****" + key[len(key)-4:]
	return key, hint, nil
}
