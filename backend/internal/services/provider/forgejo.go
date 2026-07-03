package provider

// ForgejoProvider 复用 GiteaProvider，因为 Forgejo 的 Release API 与 Gitea 兼容

import "net/http"

type ForgejoProvider struct {
	*GiteaProvider
}

func NewForgejoProvider(apiBaseURL string) *ForgejoProvider {
	return NewForgejoProviderWithTransport(apiBaseURL, nil)
}

// NewForgejoProviderWithTransport 创建带可选代理 transport 的 ForgejoProvider
func NewForgejoProviderWithTransport(apiBaseURL string, transport *http.Transport) *ForgejoProvider {
	return &ForgejoProvider{
		GiteaProvider: NewGiteaProviderWithTransport(apiBaseURL, transport),
	}
}

func (f *ForgejoProvider) Name() string {
	return "forgejo"
}
