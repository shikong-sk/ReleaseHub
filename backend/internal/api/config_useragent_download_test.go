package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TestSyncDownloadUsesConfiguredUserAgent 端到端验证：
// 配置下载 UA 后，实际下载请求携带该 UA（config → asset.Service → HTTPDownloader 全链路）。
func TestSyncDownloadUsesConfiguredUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileContent := []byte("user-agent e2e payload")
	var gotUA string
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 记录下载请求头，用于断言 UA 已生效
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write(fileContent)
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 9101,
			"tag_name": "v9.1.0",
			"name": "UA Test",
			"html_url": "https://github.com/acme/uatest/releases/tag/v9.1.0",
			"url": "https://api.github.com/repos/acme/uatest/releases/9101",
			"published_at": "2026-06-02T10:00:00Z",
			"assets": [
				{
					"id": 9201,
					"name": "uatest_linux_amd64.tar.gz",
					"size": 22,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/asset-api",
					"browser_download_url": "` + downloadServer.URL + `/asset"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	db := newTestDB(t)
	if err := database.SeedDefaultStorage(db, storageDir); err != nil {
		t.Fatalf("初始化默认存储失败: %v", err)
	}
	_ = database.BackfillAssetStorageID(db)

	const wantUA = "Mozilla/5.0 (TestAgent) ReleaseHubTest/1.0"
	router := NewRouter(Dependencies{
		Config: &config.Config{
			App:    config.AppConfig{Env: "test"},
			GitHub: config.GitHubConfig{APIBaseURL: githubServer.URL},
			Storage: config.StorageConfig{DataDir: storageDir},
			Download: config.DownloadConfig{
				UserAgentPreset: config.UserAgentPresetCustom,
				UserAgent:       wantUA,
			},
		},
		DB:     db,
		Logger: zap.NewNop(),
	})

	// 创建仓库（默认过滤模式 glob，包含全部资产）
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "uatest",
		"filterMode": "glob",
		"assetIncludePatterns": "*"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("解析仓库 ID 失败: %v, body=%s", err, createRec.Body.String())
	}

	syncPath := "/api/repositories/" + itoa(created.ID) + "/sync"
	syncRec := performRequest(router, http.MethodPost, syncPath, nil)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("同步失败: %d %s", syncRec.Code, syncRec.Body.String())
	}

	// 同步是异步执行的：轮询任务状态直到终态，最多等 5 秒
	var syncTask struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(syncRec.Body.Bytes(), &syncTask); err != nil {
		t.Fatalf("解析同步任务失败: %v, body=%s", err, syncRec.Body.String())
	}
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		taskRec := performRequest(router, http.MethodGet, "/api/tasks/"+itoa(syncTask.ID), nil)
		if taskRec.Code == http.StatusOK {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(taskRec.Body.Bytes(), &task) == nil && (task.Status == "succeeded" || task.Status == "failed") {
				break
			}
		}
	}

	if gotUA != wantUA {
		t.Fatalf("下载请求 UA 与配置不符\n期望: %s\n实际: %s", wantUA, gotUA)
	}
}

// itoa 轻量整数转字符串，避免为单处使用引入额外 import
func itoa(v uint) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}


// TestRepositoryUserAgentOverridesGlobal 验证仓库级 UA 覆盖全局配置：
// 全局设 chrome-win，仓库设自定义 UA，下载请求应使用仓库值。
func TestRepositoryUserAgentOverridesGlobal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fileContent := []byte("repo-level ua payload")
	var gotUA string
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write(fileContent)
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 9301,
			"tag_name": "v9.3.0",
			"name": "Repo UA",
			"html_url": "https://github.com/acme/repoua/releases/tag/v9.3.0",
			"url": "https://api.github.com/repos/acme/repoua/releases/9301",
			"published_at": "2026-06-02T10:00:00Z",
			"assets": [
				{
					"id": 9401,
					"name": "repoua_linux_amd64.tar.gz",
					"size": 20,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/asset-api",
					"browser_download_url": "` + downloadServer.URL + `/asset"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	db := newTestDB(t)
	if err := database.SeedDefaultStorage(db, storageDir); err != nil {
		t.Fatalf("初始化默认存储失败: %v", err)
	}
	_ = database.BackfillAssetStorageID(db)

	router := NewRouter(Dependencies{
		Config: &config.Config{
			App:     config.AppConfig{Env: "test"},
			GitHub:  config.GitHubConfig{APIBaseURL: githubServer.URL},
			Storage: config.StorageConfig{DataDir: storageDir},
			// 全局使用 Chrome 预设
			Download: config.DownloadConfig{UserAgentPreset: "chrome-win"},
		},
		DB:     db,
		Logger: zap.NewNop(),
	})

	const repoUA = "RepoLevelAgent/9.3"
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "repoua",
		"filterMode": "glob",
		"assetIncludePatterns": "*",
		"downloadUserAgentPreset": "custom",
		"downloadUserAgent": "`+repoUA+`"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("解析仓库 ID 失败: %v, body=%s", err, createRec.Body.String())
	}

	syncRec := performRequest(router, http.MethodPost, "/api/repositories/"+itoa(created.ID)+"/sync", nil)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("同步失败: %d %s", syncRec.Code, syncRec.Body.String())
	}
	var syncTask struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(syncRec.Body.Bytes(), &syncTask)
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		taskRec := performRequest(router, http.MethodGet, "/api/tasks/"+itoa(syncTask.ID), nil)
		if taskRec.Code == http.StatusOK {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(taskRec.Body.Bytes(), &task) == nil && (task.Status == "succeeded" || task.Status == "failed") {
				break
			}
		}
	}

	if gotUA != repoUA {
		t.Fatalf("下载请求 UA 应使用仓库级覆盖值\n期望: %s\n实际: %s", repoUA, gotUA)
	}
}


// TestRepositoryUserAgentCustomEmptyFallsBackToGlobal 验证「选自定义但不填值」回退全局 UA，
// 保持「留空即跟随全局」的一致语义。
func TestRepositoryUserAgentCustomEmptyFallsBackToGlobal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotUA string
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("payload"))
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 9501, "tag_name": "v9.5.0", "name": "UA fallback",
			"html_url": "https://github.com/acme/uafb/releases/tag/v9.5.0",
			"url": "https://api.github.com/repos/acme/uafb/releases/9501",
			"published_at": "2026-06-02T10:00:00Z",
			"assets": [{
				"id": 9601, "name": "uafb.bin", "size": 7, "content_type": "application/octet-stream",
				"url": "` + downloadServer.URL + `/asset-api",
				"browser_download_url": "` + downloadServer.URL + `/asset"
			}]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	db := newTestDB(t)
	if err := database.SeedDefaultStorage(db, storageDir); err != nil {
		t.Fatalf("初始化默认存储失败: %v", err)
	}
	_ = database.BackfillAssetStorageID(db)

	router := NewRouter(Dependencies{
		Config: &config.Config{
			App:     config.AppConfig{Env: "test"},
			GitHub:  config.GitHubConfig{APIBaseURL: githubServer.URL},
			Storage: config.StorageConfig{DataDir: storageDir},
			// 全局 Firefox 预设，仓库 custom+空值应回退到这个值
			Download: config.DownloadConfig{UserAgentPreset: "firefox-win"},
		},
		DB:     db,
		Logger: zap.NewNop(),
	})

	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme", "repo": "uafb",
		"filterMode": "glob", "assetIncludePatterns": "*",
		"downloadUserAgentPreset": "custom",
		"downloadUserAgent": ""
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("解析仓库 ID 失败: %v", err)
	}

	syncRec := performRequest(router, http.MethodPost, "/api/repositories/"+itoa(created.ID)+"/sync", nil)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("同步失败: %d %s", syncRec.Code, syncRec.Body.String())
	}
	var syncTask struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(syncRec.Body.Bytes(), &syncTask)
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		taskRec := performRequest(router, http.MethodGet, "/api/tasks/"+itoa(syncTask.ID), nil)
		if taskRec.Code == http.StatusOK {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(taskRec.Body.Bytes(), &task) == nil && (task.Status == "succeeded" || task.Status == "failed") {
				break
			}
		}
	}

	wantUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0"
	if gotUA != wantUA {
		t.Fatalf("仓库 custom 空值应回退全局 Firefox UA\n期望: %s\n实际: %s", wantUA, gotUA)
	}
}
