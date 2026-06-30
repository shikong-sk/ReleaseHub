package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {

	App       AppConfig
	HTTP      HTTPConfig
	Database  DatabaseConfig
	Storage   StorageConfig
	Download  DownloadConfig
	GitHub    GitHubConfig
	Scheduler SchedulerConfig
	Auth      AuthConfig
}

type AppConfig struct {
	Name      string
	Env       string
	JWTSecret string
}

type HTTPConfig struct {
	Host string
	Port int
}

func (c HTTPConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type DatabaseConfig struct {
	Driver string
	DSN    string
}

type StorageConfig struct {
	DataDir string
}

type DownloadConfig struct {
	MaxSpeedBytes int64  // 下载速度限制（字节/秒），0=不限
	Aria2RPC      string // aria2 JSON-RPC 端点，空=不使用 aria2
	Aria2Secret   string // aria2 RPC 密钥
	Aria2HTTP     string // aria2 文件服务地址
}

type GitHubConfig struct {
	APIBaseURL string
}

type SchedulerConfig struct {
	Enabled       bool
	TickSeconds   int
	MaxConcurrent int
}

type AuthConfig struct {
	Enabled         bool
	DefaultAdmin    string
	DefaultPassword string
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("RELEASEHUB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("app.name", "ReleaseHub")
	v.SetDefault("app.env", "development")
	v.SetDefault("app.jwt_secret", "")
	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "data/releasehub.db")
	v.SetDefault("storage.data_dir", "data/releases")
	v.SetDefault("download.max_speed_bytes", 0)
	v.SetDefault("download.aria2_rpc", "")
	v.SetDefault("download.aria2_secret", "")
	v.SetDefault("download.aria2_http", "")
	v.SetDefault("github.api_base_url", "https://api.github.com")
	v.SetDefault("scheduler.enabled", true)
	v.SetDefault("scheduler.tick_seconds", 60)
	v.SetDefault("scheduler.max_concurrent", 5)
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.default_admin", "admin")
	v.SetDefault("auth.default_password", "admin")

	cfg := &Config{
		App: AppConfig{
			Name:      v.GetString("app.name"),
			Env:       v.GetString("app.env"),
			JWTSecret: v.GetString("app.jwt_secret"),
		},
		HTTP: HTTPConfig{
			Host: v.GetString("http.host"),
			Port: v.GetInt("http.port"),
		},
		Database: DatabaseConfig{
			Driver: v.GetString("database.driver"),
			DSN:    v.GetString("database.dsn"),
		},
		Storage: StorageConfig{
			DataDir: v.GetString("storage.data_dir"),
		},
		Download: DownloadConfig{
			MaxSpeedBytes: v.GetInt64("download.max_speed_bytes"),
			Aria2RPC:      v.GetString("download.aria2_rpc"),
			Aria2Secret:   v.GetString("download.aria2_secret"),
			Aria2HTTP:     v.GetString("download.aria2_http"),
		},
		GitHub: GitHubConfig{
			APIBaseURL: v.GetString("github.api_base_url"),
		},
		Scheduler: SchedulerConfig{
			Enabled:       v.GetBool("scheduler.enabled"),
			TickSeconds:   v.GetInt("scheduler.tick_seconds"),
			MaxConcurrent: v.GetInt("scheduler.max_concurrent"),
		},
		Auth: AuthConfig{
			Enabled:         v.GetBool("auth.enabled"),
			DefaultAdmin:    v.GetString("auth.default_admin"),
			DefaultPassword: v.GetString("auth.default_password"),
		},
	}

	if cfg.Database.Driver != "sqlite" {
		return nil, fmt.Errorf("暂不支持数据库类型: %s", cfg.Database.Driver)
	}
	if cfg.Scheduler.TickSeconds < 10 {
		return nil, fmt.Errorf("scheduler.tick_seconds 不能小于 10")
	}
	if cfg.Scheduler.MaxConcurrent < 1 {
		return nil, fmt.Errorf("scheduler.max_concurrent 不能小于 1")
	}

	return cfg, nil
}

// UpdateConfig 定义可运行时更新的配置项
type UpdateConfig struct {
	SchedulerEnabled       *bool   `json:"schedulerEnabled,omitempty"`
	SchedulerTickSeconds   *int    `json:"schedulerTickSeconds,omitempty"`
	SchedulerMaxConcurrent *int    `json:"schedulerMaxConcurrent,omitempty"`
	GitHubAPIBaseURL       *string `json:"githubApiBaseUrl,omitempty"`
	AuthEnabled            *bool   `json:"authEnabled,omitempty"`
}

// ApplyUpdate 应用运行时配置更新，返回实际被修改的字段名列表
func (c *Config) ApplyUpdate(update UpdateConfig) ([]string, error) {
	var changed []string

	if update.SchedulerEnabled != nil && *update.SchedulerEnabled != c.Scheduler.Enabled {
		c.Scheduler.Enabled = *update.SchedulerEnabled
		changed = append(changed, "schedulerEnabled")
	}
	if update.SchedulerTickSeconds != nil {
		if *update.SchedulerTickSeconds < 10 {
			return nil, fmt.Errorf("scheduler.tick_seconds 不能小于 10")
		}
		if *update.SchedulerTickSeconds != c.Scheduler.TickSeconds {
			c.Scheduler.TickSeconds = *update.SchedulerTickSeconds
			changed = append(changed, "schedulerTickSeconds")
		}
	}
	if update.SchedulerMaxConcurrent != nil {
		if *update.SchedulerMaxConcurrent < 1 {
			return nil, fmt.Errorf("scheduler.max_concurrent 不能小于 1")
		}
		if *update.SchedulerMaxConcurrent != c.Scheduler.MaxConcurrent {
			c.Scheduler.MaxConcurrent = *update.SchedulerMaxConcurrent
			changed = append(changed, "schedulerMaxConcurrent")
		}
	}
	if update.GitHubAPIBaseURL != nil && *update.GitHubAPIBaseURL != c.GitHub.APIBaseURL {
		c.GitHub.APIBaseURL = *update.GitHubAPIBaseURL
		changed = append(changed, "githubApiBaseUrl")
	}
	if update.AuthEnabled != nil && *update.AuthEnabled != c.Auth.Enabled {
		c.Auth.Enabled = *update.AuthEnabled
		changed = append(changed, "authEnabled")
	}

	return changed, nil
}
