package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

// ClientFactory 根据代理配置创建 GitHub 客户端
type ClientFactory struct {
	apiBaseURL string
	db         *gorm.DB
}

// NewClientFactory 创建客户端工厂
func NewClientFactory(apiBaseURL string, db *gorm.DB) *ClientFactory {
	return &ClientFactory{
		apiBaseURL: apiBaseURL,
		db:         db,
	}
}

// DefaultClient 返回无代理的默认客户端
func (f *ClientFactory) DefaultClient() (*Client, error) {
	return NewClient(f.apiBaseURL)
}

// ClientForRepository 根据仓库的代理配置创建对应客户端
func (f *ClientFactory) ClientForRepository(ctx context.Context, repository models.Repository) (*Client, error) {
	if repository.ProxyID == nil {
		return f.DefaultClient()
	}

	var proxy models.Proxy
	if err := f.db.WithContext(ctx).First(&proxy, *repository.ProxyID).Error; err != nil {
		return nil, fmt.Errorf("查询代理配置失败: %w", err)
	}

	proxyURL, err := buildProxyURL(proxy)
	if err != nil {
		return nil, fmt.Errorf("代理配置无效: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	return NewClientWithTransport(f.apiBaseURL, transport)
}

// TransportForRepository 根据仓库代理配置返回 http.Transport
func (f *ClientFactory) TransportForRepository(ctx context.Context, repository models.Repository) (*http.Transport, error) {
	if repository.ProxyID == nil {
		return &http.Transport{}, nil
	}

	var proxy models.Proxy
	if err := f.db.WithContext(ctx).First(&proxy, *repository.ProxyID).Error; err != nil {
		return nil, fmt.Errorf("查询代理配置失败: %w", err)
	}

	proxyURL, err := buildProxyURL(proxy)
	if err != nil {
		return nil, fmt.Errorf("代理配置无效: %w", err)
	}

	return &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}, nil
}

func buildProxyURL(proxy models.Proxy) (*url.URL, error) {
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
