package provider

// ForgejoProvider 复用 GiteaProvider，因为 Forgejo 的 Release API 与 Gitea 兼容
type ForgejoProvider struct {
	*GiteaProvider
}

func NewForgejoProvider(apiBaseURL string) *ForgejoProvider {
	return &ForgejoProvider{
		GiteaProvider: NewGiteaProvider(apiBaseURL),
	}
}

func (f *ForgejoProvider) Name() string {
	return "forgejo"
}
