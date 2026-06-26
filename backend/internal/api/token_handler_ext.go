package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type rateLimitResponse struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	ResetAt   int64 `json:"resetAt"`
	Used      int   `json:"used"`
}

type tokenHealthResponse struct {
	Valid     bool              `json:"valid"`
	RateLimit *rateLimitResponse `json:"rateLimit,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// registerTokenHealthRoutes 注册 Token 健康检查路由
func registerTokenHealthRoutes(router *gin.Engine, db *gorm.DB, apiBaseURL string) {
	router.GET("/api/tokens/:id/health", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var token models.GitHubToken
		if err := db.WithContext(c.Request.Context()).First(&token, id).Error; err != nil {
			if isNotFound(err) {
				writeError(c, http.StatusNotFound, "Token 不存在")
				return
			}
			writeError(c, http.StatusInternalServerError, "查询 Token 失败")
			return
		}
		result := checkTokenHealth(c.Request.Context(), token.Token, apiBaseURL)
		c.JSON(http.StatusOK, result)
	})

	router.GET("/api/tokens/:id/rate-limit", func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var token models.GitHubToken
		if err := db.WithContext(c.Request.Context()).First(&token, id).Error; err != nil {
			if isNotFound(err) {
				writeError(c, http.StatusNotFound, "Token 不存在")
				return
			}
			writeError(c, http.StatusInternalServerError, "查询 Token 失败")
			return
		}
		result := checkTokenRateLimit(c.Request.Context(), token.Token, apiBaseURL)
		c.JSON(http.StatusOK, result)
	})
}

func checkTokenHealth(ctx context.Context, token string, apiBaseURL string) tokenHealthResponse {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/rate_limit", nil)
	if err != nil {
		return tokenHealthResponse{Valid: false, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "ReleaseHub-TokenHealth")

	resp, err := client.Do(req)
	if err != nil {
		return tokenHealthResponse{Valid: false, Error: "连接失败: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return tokenHealthResponse{Valid: false, Error: "Token 无效或已过期"}
	}
	if resp.StatusCode == http.StatusForbidden {
		return tokenHealthResponse{Valid: false, Error: "Token 权限不足"}
	}

	result := tokenHealthResponse{Valid: true}
	if rl := parseRateLimitHeaders(resp); rl != nil {
		result.RateLimit = rl
	}
	return result
}

func checkTokenRateLimit(ctx context.Context, token string, apiBaseURL string) *rateLimitResponse {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/rate_limit", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "ReleaseHub-RateLimit")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	// 尝试从响应头解析
	if rl := parseRateLimitHeaders(resp); rl != nil {
		return rl
	}

	// 尝试从响应体解析
	var body struct {
		Resources struct {
			Core struct {
				Limit     int `json:"limit"`
				Remaining int `json:"remaining"`
				Reset     int64 `json:"reset"`
				Used      int `json:"used"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.Resources.Core.Limit > 0 {
		return &rateLimitResponse{
			Limit:     body.Resources.Core.Limit,
			Remaining: body.Resources.Core.Remaining,
			ResetAt:   body.Resources.Core.Reset,
			Used:      body.Resources.Core.Used,
		}
	}
	return nil
}

func parseRateLimitHeaders(resp *http.Response) *rateLimitResponse {
	limit, err1 := strconv.Atoi(resp.Header.Get("X-RateLimit-Limit"))
	remaining, err2 := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	reset, err3 := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)

	if err1 != nil || err2 != nil || err3 != nil || limit == 0 {
		return nil
	}

	return &rateLimitResponse{
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   reset,
		Used:      limit - remaining,
	}
}

func isNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
