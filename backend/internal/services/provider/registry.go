package provider

import (
	"fmt"
	"strings"
)

// Registry 按 provider 名称和 API base URL 创建 ReleaseProvider 实例
type Registry struct {
	githubAPIBaseURL string
}

func NewRegistry(githubAPIBaseURL string) *Registry {
	return &Registry{githubAPIBaseURL: githubAPIBaseURL}
}

// SupportedProviders 返回当前支持的 provider 名称列表
func SupportedProviders() []string {
	return []string{"github", "gitlab", "gitea", "forgejo"}
}

// IsSupported 判断 provider 名称是否受支持
func IsSupported(name string) bool {
	for _, p := range SupportedProviders() {
		if p == name {
			return true
		}
	}
	return false
}

// GetProvider 根据仓库的 provider 和可选的 API base URL 创建对应的 ReleaseProvider
// apiBaseURL 为空时使用默认值
func (r *Registry) GetProvider(providerName, apiBaseURL string) (ReleaseProvider, error) {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "github", "":
		base := apiBaseURL
		if base == "" {
			base = r.githubAPIBaseURL
		}
		client, err := newGitHubClient(base)
		if err != nil {
			return nil, err
		}
		return NewGitHubProvider(client), nil
	case "gitlab":
		return NewGitLabProvider(apiBaseURL), nil
	case "gitea":
		return NewGiteaProvider(apiBaseURL), nil
	case "forgejo":
		return NewForgejoProvider(apiBaseURL), nil
	default:
		return nil, fmt.Errorf("不支持的 provider: %s", providerName)
	}
}
