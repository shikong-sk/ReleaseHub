package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

func TransportForRepository(ctx context.Context, db *gorm.DB, repository models.Repository) (*http.Transport, error) {
	return TransportForID(ctx, db, repository.ProxyID)
}

func TransportForID(ctx context.Context, db *gorm.DB, proxyID *uint) (*http.Transport, error) {
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

	proxyURL, err := BuildURL(proxy)
	if err != nil {
		return nil, err
	}
	return &http.Transport{Proxy: http.ProxyURL(proxyURL)}, nil
}

func BuildURL(proxy models.Proxy) (*url.URL, error) {
	scheme := "http"
	switch proxy.Type {
	case "https":
		scheme = "https"
	case "socks5":
		scheme = "socks5"
	case "http", "":
		scheme = "http"
	default:
		return nil, fmt.Errorf("不支持的代理类型: %s", proxy.Type)
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
