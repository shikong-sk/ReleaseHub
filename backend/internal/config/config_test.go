package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载默认配置失败: %v", err)
	}

	if cfg.App.Name != "ReleaseHub" {
		t.Fatalf("默认应用名不正确: %s", cfg.App.Name)
	}
	if cfg.HTTP.Port != 8080 {
		t.Fatalf("默认端口不正确: %d", cfg.HTTP.Port)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("默认数据库类型不正确: %s", cfg.Database.Driver)
	}
	if !cfg.Scheduler.Enabled {
		t.Fatal("默认应启用 Scheduler")
	}
	if cfg.Scheduler.TickSeconds != 60 {
		t.Fatalf("默认 Scheduler 扫描间隔不正确: %d", cfg.Scheduler.TickSeconds)
	}
	if cfg.Scheduler.MaxConcurrent != 5 {
		t.Fatalf("默认 Scheduler 并发数不正确: %d", cfg.Scheduler.MaxConcurrent)
	}
}

func TestLoadRejectsSchedulerTickBelowMinimum(t *testing.T) {
	t.Setenv("RELEASEHUB_SCHEDULER_TICK_SECONDS", "5")

	if _, err := Load(); err == nil {
		t.Fatal("期望过短 Scheduler 扫描间隔返回错误")
	}
}

func TestLoadRejectsSchedulerMaxConcurrentBelowMinimum(t *testing.T) {
	t.Setenv("RELEASEHUB_SCHEDULER_MAX_CONCURRENT", "0")

	if _, err := Load(); err == nil {
		t.Fatal("期望 Scheduler 并发数过小返回错误")
	}
}

func TestLoadSchedulerMaxConcurrentFromEnv(t *testing.T) {
	t.Setenv("RELEASEHUB_SCHEDULER_MAX_CONCURRENT", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Scheduler.MaxConcurrent != 10 {
		t.Fatalf("期望并发数 10，实际 %d", cfg.Scheduler.MaxConcurrent)
	}
}
