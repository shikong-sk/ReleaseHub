package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type proxyHandler struct {
	db *gorm.DB
}

type proxyInput struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Type     string `json:"type" binding:"required,oneof=http https socks5"`
	Host     string `json:"host" binding:"required,min=1,max=512"`
	Port     int    `json:"port" binding:"required,min=1,max=65535"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type proxyResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func registerProxyRoutes(router *gin.Engine, db *gorm.DB) {
	handler := proxyHandler{db: db}
	group := router.Group("/api/proxies")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:id", handler.get)
	group.PATCH("/:id", handler.update)
	group.DELETE("/:id", handler.delete)
	group.POST("/:id/test", handler.testConnection)
}

func (h proxyHandler) list(c *gin.Context) {
	var proxies []models.Proxy
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&proxies).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询代理列表失败")
		return
	}
	items := make([]proxyResponse, 0, len(proxies))
	for _, p := range proxies {
		items = append(items, toProxyResponse(p))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h proxyHandler) create(c *gin.Context) {
	var input proxyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}
	proxy := models.Proxy{
		Name:     input.Name,
		Type:     strings.ToLower(input.Type),
		Host:     input.Host,
		Port:     input.Port,
		Username: input.Username,
		Password: input.Password,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&proxy).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建代理失败")
		return
	}
	c.JSON(http.StatusCreated, toProxyResponse(proxy))
}

func (h proxyHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var proxy models.Proxy
	if err := h.db.WithContext(c.Request.Context()).First(&proxy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "代理不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询代理失败")
		return
	}
	c.JSON(http.StatusOK, toProxyResponse(proxy))
}

func (h proxyHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var proxy models.Proxy
	if err := h.db.WithContext(c.Request.Context()).First(&proxy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "代理不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询代理失败")
		return
	}
	var input proxyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}
	proxy.Name = input.Name
	proxy.Type = strings.ToLower(input.Type)
	proxy.Host = input.Host
	proxy.Port = input.Port
	proxy.Username = input.Username
	if input.Password != "" {
		proxy.Password = input.Password
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&proxy).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "更新代理失败")
		return
	}
	c.JSON(http.StatusOK, toProxyResponse(proxy))
}

func (h proxyHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var proxy models.Proxy
	if err := h.db.WithContext(c.Request.Context()).First(&proxy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "代理不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询代理失败")
		return
	}
	var count int64
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).
		Where("proxy_id = ? AND proxy_id IS NOT NULL", id).Count(&count)
	if count > 0 {
		writeError(c, http.StatusConflict, fmt.Sprintf("该代理正在被 %d 个仓库使用，无法删除", count))
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Delete(&proxy).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除代理失败")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h proxyHandler) testConnection(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var proxy models.Proxy
	if err := h.db.WithContext(c.Request.Context()).First(&proxy, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "代理不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询代理失败")
		return
	}
	proxyURL, err := BuildProxyURL(proxy)
	if err != nil {
		writeError(c, http.StatusBadRequest, "代理配置无效: "+err.Error())
		return
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   15 * time.Second,
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, "https://api.github.com/rate_limit", nil)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "创建测试请求失败")
		return
	}
	req.Header.Set("User-Agent", "ReleaseHub-ProxyTest")
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		writeError(c, http.StatusBadGateway, fmt.Sprintf("代理连接失败: %s (耗时 %s)", err.Error(), elapsed.Round(time.Millisecond)))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(c, http.StatusBadGateway, fmt.Sprintf("代理连接异常: HTTP %d (耗时 %s)", resp.StatusCode, elapsed.Round(time.Millisecond)))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": fmt.Sprintf("代理连接成功 (耗时 %s)", elapsed.Round(time.Millisecond)),
	})
}

// BuildProxyURL 根据代理配置构建 *url.URL
func BuildProxyURL(proxy models.Proxy) (*url.URL, error) {
	scheme := "http"
	switch proxy.Type {
	case "https":
		scheme = "https"
	case "socks5":
		scheme = "socks5"
	}
	proxyURL := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", proxy.Host, proxy.Port),
	}
	if proxy.Username != "" {
		proxyURL.User = url.UserPassword(proxy.Username, proxy.Password)
	}
	return proxyURL, nil
}

// GetProxyTransport 根据代理 ID 构建带代理的 http.Transport
func GetProxyTransport(ctx context.Context, db *gorm.DB, proxyID *uint) (*http.Transport, error) {
	if proxyID == nil {
		return &http.Transport{}, nil
	}
	var proxy models.Proxy
	if err := db.WithContext(ctx).First(&proxy, *proxyID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("代理不存在 (id=%d)", *proxyID)
		}
		return nil, err
	}
	proxyURL, err := BuildProxyURL(proxy)
	if err != nil {
		return nil, err
	}
	return &http.Transport{Proxy: http.ProxyURL(proxyURL)}, nil
}

func toProxyResponse(p models.Proxy) proxyResponse {
	return proxyResponse{
		ID:        p.ID,
		Name:      p.Name,
		Type:      p.Type,
		Host:      p.Host,
		Port:      p.Port,
		Username:  p.Username,
		CreatedAt: p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
