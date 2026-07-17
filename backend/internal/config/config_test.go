package config

import (
	"reflect"
	"slices"
	"testing"
)

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

// === Phase 6: ApplyUpdate 单测（aria2 + 限速字段）===

func strPtr(s string) *string { return &s }

func i64Ptr(i int64) *int64 { return &i }

// TestApplyUpdateAria2Fields 覆盖 aria2RPC/Secret/HTTP/Dir 四字段四态：
// enable（空→值）、disable（值→空）、nil 不动、same 不动。
// 空串合法：RPC 空为禁用 aria2 回 HTTP；Dir 空表示用 daemon 默认目录。无归一化（普通字符串比较）。
func TestApplyUpdateAria2Fields(t *testing.T) {
	states := []struct {
		name       string
		initial    string
		update     *string
		wantChange bool
		wantValue  string
	}{
		{"enable_from_empty", "", strPtr("set"), true, "set"},
		{"disable_to_empty", "old", strPtr(""), true, ""},
		{"nil_no_change", "old", nil, false, "old"},
		{"same_value", "old", strPtr("old"), false, "old"},
	}
	fields := []struct {
		name string
		key  string
		set  func(d *DownloadConfig, v string)
		setU func(u *UpdateConfig, v *string)
		get  func(d *DownloadConfig) string
	}{
		{"Aria2RPC", "aria2RPC",
			func(d *DownloadConfig, v string) { d.Aria2RPC = v },
			func(u *UpdateConfig, v *string) { u.Aria2RPC = v },
			func(d *DownloadConfig) string { return d.Aria2RPC }},
		{"Aria2Secret", "aria2Secret",
			func(d *DownloadConfig, v string) { d.Aria2Secret = v },
			func(u *UpdateConfig, v *string) { u.Aria2Secret = v },
			func(d *DownloadConfig) string { return d.Aria2Secret }},
		{"Aria2HTTP", "aria2HTTP",
			func(d *DownloadConfig, v string) { d.Aria2HTTP = v },
			func(u *UpdateConfig, v *string) { u.Aria2HTTP = v },
			func(d *DownloadConfig) string { return d.Aria2HTTP }},
		{"Aria2Dir", "aria2Dir",
			func(d *DownloadConfig, v string) { d.Aria2Dir = v },
			func(u *UpdateConfig, v *string) { u.Aria2Dir = v },
			func(d *DownloadConfig) string { return d.Aria2Dir }},
	}
	for _, f := range fields {
		for _, st := range states {
			t.Run(f.name+"_"+st.name, func(t *testing.T) {
				c := &Config{Download: DownloadConfig{}}
				f.set(&c.Download, st.initial)
				uc := UpdateConfig{}
				if st.update != nil {
					f.setU(&uc, st.update)
				}
				changed, _ := c.ApplyUpdate(uc)
				if got := f.get(&c.Download); got != st.wantValue {
					t.Fatalf("%s/%s: 值期望 %q 实际 %q", f.name, st.name, st.wantValue, got)
				}
				if slices.Contains(changed, f.key) != st.wantChange {
					t.Fatalf("%s/%s: changed 含 %q 期望 %v 实际 %v", f.name, st.name, f.key, st.wantChange, changed)
				}
			})
		}
	}
}

// TestApplyUpdateDownloadMaxSpeedBytes 覆盖限速字段：启用、禁用、<0 归一化为 0、nil、same。
// <0 归一化后若等于现值则不写 changed（避免误报）。
func TestApplyUpdateDownloadMaxSpeedBytes(t *testing.T) {
	twoMB := int64(2 * 1024 * 1024)
	cases := []struct {
		name       string
		initial    int64
		update     *int64
		wantChange bool
		wantValue  int64
	}{
		{"enable_from_zero", 0, i64Ptr(twoMB), true, twoMB},
		{"disable_to_zero", twoMB, i64Ptr(0), true, 0},
		{"negative_normalized_no_change", 0, i64Ptr(-1), false, 0},   // -1 → 0；0 == 0 现值不变
		{"negative_normalized_with_change", 5, i64Ptr(-1), true, 0},  // -1 → 0；0 ≠ 5 变更
		{"nil_no_change", 5, nil, false, 5},
		{"same_value", 5, i64Ptr(5), false, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Download: DownloadConfig{MaxSpeedBytes: tc.initial}}
			uc := UpdateConfig{}
			if tc.update != nil {
				uc.DownloadMaxSpeedBytes = tc.update
			}
			changed, _ := c.ApplyUpdate(uc)
			if c.Download.MaxSpeedBytes != tc.wantValue {
				t.Fatalf("%s: MaxSpeedBytes 期望 %d 实际 %d", tc.name, tc.wantValue, c.Download.MaxSpeedBytes)
			}
			if slices.Contains(changed, "downloadMaxSpeedBytes") != tc.wantChange {
				t.Fatalf("%s: changed 含 downloadMaxSpeedBytes 期望 %v 实际 %v", tc.name, tc.wantChange, changed)
			}
		})
	}
}

// TestApplyUpdateMultipleFields 验证一次性多字段 Update 的 changed 全序：
// 顺序固化与 ApplyUpdate 各 if 块声明顺序同步，防止后续追加新 if 块时回归。
func TestApplyUpdateMultipleFields(t *testing.T) {
	c := &Config{Download: DownloadConfig{}}
	twoMB := int64(2 * 1024 * 1024)
	rpc := "http://a:6800/jsonrpc"
	secret := "topsecret"
	httpEP := "http://a:6801/"
	dir := "/data/aria2"
	uc := UpdateConfig{
		DownloadMaxSpeedBytes: &twoMB,
		Aria2RPC:              &rpc,
		Aria2Secret:           &secret,
		Aria2HTTP:             &httpEP,
		Aria2Dir:              &dir,
	}
	want := []string{"downloadMaxSpeedBytes", "aria2RPC", "aria2Secret", "aria2HTTP", "aria2Dir"}
	changed, _ := c.ApplyUpdate(uc)
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed 顺序/内容期望 %v 实际 %v", want, changed)
	}
	if c.Download.MaxSpeedBytes != twoMB {
		t.Fatalf("MaxSpeedBytes 期望 %d 实际 %d", twoMB, c.Download.MaxSpeedBytes)
	}
	if c.Download.Aria2RPC != rpc {
		t.Fatalf("Aria2RPC 期望 %q 实际 %q", rpc, c.Download.Aria2RPC)
	}
	if c.Download.Aria2Secret != secret {
		t.Fatalf("Aria2Secret 期望 %q 实际 %q", secret, c.Download.Aria2Secret)
	}
	if c.Download.Aria2HTTP != httpEP {
		t.Fatalf("Aria2HTTP 期望 %q 实际 %q", httpEP, c.Download.Aria2HTTP)
	}
	if c.Download.Aria2Dir != dir {
		t.Fatalf("Aria2Dir 期望 %q 实际 %q", dir, c.Download.Aria2Dir)
	}
}
