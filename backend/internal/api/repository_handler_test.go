package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"
	releasesvc "releasehub/backend/internal/services/release"
	syncersvc "releasehub/backend/internal/services/syncer"

	"go.uber.org/zap"
)

func TestRepositoryCRUD(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	createBody := []byte(`{
		"owner": "hashicorp",
		"repo": "terraform",
		"filterMode": "regex",
		"assetIncludePatterns": ".*linux.*amd64.*",
		"intervalSeconds": 900,
		"retentionKeepLatest": 3
	}`)

	createRec := performRequest(router, http.MethodPost, "/api/repositories", createBody)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("期望创建状态码 201，实际 %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID                  uint   `json:"id"`
		Provider            string `json:"provider"`
		Owner               string `json:"owner"`
		Repo                string `json:"repo"`
		Enabled             bool   `json:"enabled"`
		FilterMode          string `json:"filterMode"`
		RetentionKeepLatest int    `json:"retentionKeepLatest"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if created.ID == 0 || created.Provider != "github" || created.Owner != "hashicorp" || created.Repo != "terraform" {
		t.Fatalf("创建响应不符合预期: %+v", created)
	}
	if !created.Enabled || created.FilterMode != "regex" || created.RetentionKeepLatest != 3 {
		t.Fatalf("创建默认/输入字段不符合预期: %+v", created)
	}

	listRec := performRequest(router, http.MethodGet, "/api/repositories", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("期望列表状态码 200，实际 %d", listRec.Code)
	}

	updateBody := []byte(`{"enabled": false, "intervalSeconds": 1800}`)
	updateRec := performRequest(router, http.MethodPatch, "/api/repositories/1", updateBody)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("期望更新状态码 200，实际 %d: %s", updateRec.Code, updateRec.Body.String())
	}

	var updated struct {
		Enabled         bool `json:"enabled"`
		IntervalSeconds int  `json:"intervalSeconds"`
	}
	if err := json.Unmarshal(updateRec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("解析更新响应失败: %v", err)
	}
	if updated.Enabled || updated.IntervalSeconds != 1800 {
		t.Fatalf("更新结果不符合预期: %+v", updated)
	}

	deleteRec := performRequest(router, http.MethodDelete, "/api/repositories/1", nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("期望删除状态码 204，实际 %d", deleteRec.Code)
	}

	getDeletedRec := performRequest(router, http.MethodGet, "/api/repositories/1", nil)
	if getDeletedRec.Code != http.StatusNotFound {
		t.Fatalf("期望删除后查询状态码 404，实际 %d", getDeletedRec.Code)
	}
}

func TestRepositoryValidation(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "缺少 owner", body: `{"repo":"terraform"}`},
		{name: "无效 owner", body: `{"owner":"bad_owner","repo":"terraform"}`},
		{name: "间隔过短", body: `{"owner":"hashicorp","repo":"terraform","intervalSeconds":60}`},
		{name: "过滤模式错误", body: `{"owner":"hashicorp","repo":"terraform","filterMode":"wildcard"}`},
		{name: "保留数量错误", body: `{"owner":"hashicorp","repo":"terraform","retentionKeepLatest":-1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performRequest(router, http.MethodPost, "/api/repositories", []byte(tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("期望状态码 400，实际 %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRepositoryDuplicateReturnsConflict(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)
	body := []byte(`{"owner":"hashicorp","repo":"terraform"}`)

	firstRec := performRequest(router, http.MethodPost, "/api/repositories", body)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("期望首次创建状态码 201，实际 %d: %s", firstRec.Code, firstRec.Body.String())
	}

	secondRec := performRequest(router, http.MethodPost, "/api/repositories", body)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("期望重复创建状态码 409，实际 %d: %s", secondRec.Code, secondRec.Body.String())
	}
}

func TestRepositoryCheckLatestPersistsReleaseAndFilteredAssets(t *testing.T) {
	t.Parallel()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/hashicorp/terraform/releases/latest" {
			t.Fatalf("未预期的 GitHub API 路径: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 1001,
			"tag_name": "v1.2.3",
			"name": "Terraform v1.2.3",
			"body": "release body",
			"html_url": "https://github.com/hashicorp/terraform/releases/tag/v1.2.3",
			"url": "https://api.github.com/repos/hashicorp/terraform/releases/1001",
			"published_at": "2026-06-01T10:00:00Z",
			"assets": [
				{
					"id": 2001,
					"name": "terraform_linux_amd64.zip",
					"size": 1024,
					"content_type": "application/zip",
					"url": "https://api.github.com/assets/2001",
					"browser_download_url": "https://github.com/download/linux"
				},
				{
					"id": 2002,
					"name": "terraform_darwin_arm64.zip",
					"size": 2048,
					"content_type": "application/zip",
					"url": "https://api.github.com/assets/2002",
					"browser_download_url": "https://github.com/download/darwin"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	router := newTestRouterWithGitHubBaseURLAndStorageDir(t, githubServer.URL, storageDir)
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "hashicorp",
		"repo": "terraform",
		"filterMode": "glob",
		"assetIncludePatterns": "*linux*amd64*"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}

	checkRec := performRequest(router, http.MethodPost, "/api/repositories/1/check", nil)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("检查 release 失败: %d %s", checkRec.Code, checkRec.Body.String())
	}

	var checkBody struct {
		Release struct {
			Tag string `json:"tag"`
		} `json:"release"`
		Assets []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"assets"`
		Repository struct {
			LastReleaseTag string `json:"lastReleaseTag"`
			LastStatus     string `json:"lastStatus"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(checkRec.Body.Bytes(), &checkBody); err != nil {
		t.Fatalf("解析检查响应失败: %v", err)
	}
	if checkBody.Release.Tag != "v1.2.3" {
		t.Fatalf("期望 release tag v1.2.3，实际 %s", checkBody.Release.Tag)
	}
	if checkBody.Repository.LastReleaseTag != "v1.2.3" || checkBody.Repository.LastStatus != "healthy" {
		t.Fatalf("仓库状态未更新: %+v", checkBody.Repository)
	}
	if len(checkBody.Assets) != 2 {
		t.Fatalf("期望 2 个资产，实际 %d", len(checkBody.Assets))
	}

	statusByName := map[string]string{}
	for _, asset := range checkBody.Assets {
		statusByName[asset.Name] = asset.Status
	}
	if statusByName["terraform_linux_amd64.zip"] != "pending" {
		t.Fatalf("linux amd64 资产应为 pending，实际 %s", statusByName["terraform_linux_amd64.zip"])
	}
	if statusByName["terraform_darwin_arm64.zip"] != "skipped" {
		t.Fatalf("darwin arm64 资产应为 skipped，实际 %s", statusByName["terraform_darwin_arm64.zip"])
	}

	releasesRec := performRequest(router, http.MethodGet, "/api/repositories/1/releases", nil)
	if releasesRec.Code != http.StatusOK {
		t.Fatalf("查询 release 列表失败: %d", releasesRec.Code)
	}

	assetsRec := performRequest(router, http.MethodGet, "/api/releases/1/assets", nil)
	if assetsRec.Code != http.StatusOK {
		t.Fatalf("查询 asset 列表失败: %d", assetsRec.Code)
	}
}

func TestAssetDownloadStoresFileAndSHA256(t *testing.T) {
	t.Parallel()

	fileContent := []byte("releasehub test asset")
	fileHash := sha256.Sum256(fileContent)
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fileContent)
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 1001,
			"tag_name": "v1.0.0",
			"name": "Release v1.0.0",
			"body": "",
			"html_url": "https://github.com/acme/tool/releases/tag/v1.0.0",
			"url": "https://api.github.com/repos/acme/tool/releases/1001",
			"published_at": "2026-06-01T10:00:00Z",
			"assets": [
				{
					"id": 2001,
					"name": "tool_linux_amd64.tar.gz",
					"size": 21,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/asset-api",
					"browser_download_url": "` + downloadServer.URL + `/asset"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	router := newTestRouterWithStoppedSyncer(t, githubServer.URL, storageDir)
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "tool",
		"filterMode": "glob",
		"assetIncludePatterns": "*linux*amd64*"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}

	checkRec := performRequest(router, http.MethodPost, "/api/repositories/1/check", nil)
	if checkRec.Code != http.StatusOK {
		t.Fatalf("检查 release 失败: %d %s", checkRec.Code, checkRec.Body.String())
	}

	var checkBody struct {
		Assets []struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(checkRec.Body.Bytes(), &checkBody); err != nil {
		t.Fatalf("解析检查响应失败: %v", err)
	}
	if len(checkBody.Assets) != 1 {
		t.Fatalf("期望 1 个资产，实际 %d", len(checkBody.Assets))
	}
	assetID := checkBody.Assets[0].ID

	downloadRec := performRequest(router, http.MethodPost, "/api/assets/"+strconv.Itoa(int(assetID))+"/download", nil)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("下载资产失败: %d %s", downloadRec.Code, downloadRec.Body.String())
	}

	var downloadBody struct {
		Asset struct {
			Status      string `json:"status"`
			SHA256      string `json:"sha256"`
			StoragePath string `json:"storagePath"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(downloadRec.Body.Bytes(), &downloadBody); err != nil {
		t.Fatalf("解析下载响应失败: %v", err)
	}
	if downloadBody.Asset.Status != "verified" {
		t.Fatalf("期望资产状态 verified，实际 %s", downloadBody.Asset.Status)
	}
	if downloadBody.Asset.SHA256 != hex.EncodeToString(fileHash[:]) {
		t.Fatalf("sha256 不符合预期: %s", downloadBody.Asset.SHA256)
	}
	if downloadBody.Asset.StoragePath == "" {
		t.Fatal("storagePath 不应为空")
	}

	fileRec := performRequest(router, http.MethodGet, "/api/assets/"+strconv.Itoa(int(assetID))+"/file", nil)
	if fileRec.Code != http.StatusOK {
		t.Fatalf("读取资产文件失败: %d %s", fileRec.Code, fileRec.Body.String())
	}
	if !bytes.Equal(fileRec.Body.Bytes(), fileContent) {
		t.Fatalf("下载文件内容不符合预期: %q", fileRec.Body.String())
	}

	tasksRec := performRequest(router, http.MethodGet, "/api/tasks", nil)
	if tasksRec.Code != http.StatusOK {
		t.Fatalf("查询任务列表失败: %d %s", tasksRec.Code, tasksRec.Body.String())
	}
	if !bytes.Contains(tasksRec.Body.Bytes(), []byte("download_asset")) {
		t.Fatalf("任务列表缺少 download_asset: %s", tasksRec.Body.String())
	}

	filesRec := performRequest(router, http.MethodGet, "/api/files", nil)
	if filesRec.Code != http.StatusOK {
		t.Fatalf("查询文件列表失败: %d %s", filesRec.Code, filesRec.Body.String())
	}
	if !bytes.Contains(filesRec.Body.Bytes(), []byte("tool_linux_amd64.tar.gz")) {
		t.Fatalf("文件列表缺少已下载资产: %s", filesRec.Body.String())
	}
}

func TestRepositorySyncDownloadsMatchedAssets(t *testing.T) {
	t.Parallel()

	fileContent := []byte("releasehub sync asset")
	var downloadCount int32
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&downloadCount, 1)
		_, _ = w.Write(fileContent)
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/tool/releases/latest" {
			t.Fatalf("未预期的 GitHub API 路径: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 3001,
			"tag_name": "v2.0.0",
			"name": "Release v2.0.0",
			"body": "",
			"html_url": "https://github.com/acme/tool/releases/tag/v2.0.0",
			"url": "https://api.github.com/repos/acme/tool/releases/3001",
			"published_at": "2026-06-02T10:00:00Z",
			"assets": [
				{
					"id": 4001,
					"name": "tool_linux_amd64.tar.gz",
					"size": 21,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/asset-api",
					"browser_download_url": "` + downloadServer.URL + `/asset"
				},
				{
					"id": 4002,
					"name": "tool_windows_amd64.zip",
					"size": 22,
					"content_type": "application/zip",
					"url": "` + downloadServer.URL + `/windows-api",
					"browser_download_url": "` + downloadServer.URL + `/windows"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	router := newTestRouterWithGitHubBaseURLAndStorageDir(t, githubServer.URL, storageDir)
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "tool",
		"filterMode": "regex",
		"assetIncludePatterns": ".*linux.*amd64.*"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}

	syncRec := performRequest(router, http.MethodPost, "/api/repositories/1/sync", nil)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("同步请求失败: %d %s", syncRec.Code, syncRec.Body.String())
	}

	// 异步同步：等待后台 goroutine 完成
	var taskID uint
	var syncTaskBody struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(syncRec.Body.Bytes(), &syncTaskBody); err != nil {
		t.Fatalf("解析同步任务响应失败: %v", err)
	}
	taskID = syncTaskBody.ID

	// 轮询等待任务完成
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		taskRec := performRequest(router, http.MethodGet, fmt.Sprintf("/api/tasks/%d", taskID), nil)
		if taskRec.Code == http.StatusOK {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(taskRec.Body.Bytes(), &task) == nil && (task.Status == "succeeded" || task.Status == "failed") {
				break
			}
		}
	}
	if atomic.LoadInt32(&downloadCount) != 1 {
		t.Fatalf("应只下载 1 个匹配资产，实际下载次数 %d", downloadCount)
	}

	// 查询 Release 和资产列表验证结果
	releasesRec := performRequest(router, http.MethodGet, "/api/repositories/1/releases", nil)
	if releasesRec.Code != http.StatusOK {
		t.Fatalf("查询 Release 列表失败: %d", releasesRec.Code)
	}
	var releasesBody struct {
		Items []struct {
			ID  uint   `json:"id"`
			Tag string `json:"tag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(releasesRec.Body.Bytes(), &releasesBody); err != nil {
		t.Fatalf("解析 Release 列表失败: %v", err)
	}
	if len(releasesBody.Items) == 0 {
		t.Fatalf("期望至少 1 个 Release")
	}

	assetsRec := performRequest(router, http.MethodGet, fmt.Sprintf("/api/releases/%d/assets", releasesBody.Items[0].ID), nil)
	if assetsRec.Code != http.StatusOK {
		t.Fatalf("查询资产列表失败: %d", assetsRec.Code)
	}
	var assetsBody struct {
		Items []struct {
			ID          uint   `json:"id"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			StoragePath string `json:"storagePath"`
		} `json:"items"`
	}
	if err := json.Unmarshal(assetsRec.Body.Bytes(), &assetsBody); err != nil {
		t.Fatalf("解析资产列表失败: %v", err)
	}

	var downloadedAssetID uint
	statusByName := map[string]string{}
	storagePathByName := map[string]string{}
	for _, asset := range assetsBody.Items {
		statusByName[asset.Name] = asset.Status
		storagePathByName[asset.Name] = asset.StoragePath
		if asset.Name == "tool_linux_amd64.tar.gz" {
			downloadedAssetID = asset.ID
		}
	}
	if statusByName["tool_linux_amd64.tar.gz"] != "verified" {
		t.Fatalf("匹配资产应为 verified，实际 %s", statusByName["tool_linux_amd64.tar.gz"])
	}
	if storagePathByName["tool_linux_amd64.tar.gz"] != "github/acme/tool/v2.0.0/tool_linux_amd64.tar.gz" {
		t.Fatalf("匹配资产路径不符合预期: %s", storagePathByName["tool_linux_amd64.tar.gz"])
	}
	if statusByName["tool_windows_amd64.zip"] != "skipped" || storagePathByName["tool_windows_amd64.zip"] != "" {
		t.Fatalf("未匹配资产应被跳过且无存储路径: %+v", assetsBody.Items)
	}

	fileRec := performRequest(router, http.MethodGet, "/api/assets/"+strconv.Itoa(int(downloadedAssetID))+"/file", nil)
	if fileRec.Code != http.StatusOK {
		t.Fatalf("读取同步文件失败: %d %s", fileRec.Code, fileRec.Body.String())
	}
	if !bytes.Equal(fileRec.Body.Bytes(), fileContent) {
		t.Fatalf("同步文件内容不符合预期: %q", fileRec.Body.String())
	}
	downloadAliasRec := performRequest(router, http.MethodGet, "/api/files/download?assetId="+strconv.Itoa(int(downloadedAssetID)), nil)
	if downloadAliasRec.Code != http.StatusFound {
		t.Fatalf("文件下载别名应返回 302，实际 %d %s", downloadAliasRec.Code, downloadAliasRec.Body.String())
	}
	if downloadAliasRec.Header().Get("Location") != "/api/assets/"+strconv.Itoa(int(downloadedAssetID))+"/file" {
		t.Fatalf("文件下载别名 Location 不符合预期: %s", downloadAliasRec.Header().Get("Location"))
	}

	latestTarget, err := os.Readlink(filepath.Join(storageDir, "github", "acme", "tool", "latest"))
	if err != nil {
		t.Fatalf("读取 latest 软链接失败: %v", err)
	}
	if latestTarget != "v2.0.0" {
		t.Fatalf("latest 软链接目标不符合预期: %s", latestTarget)
	}
	latestManifest, err := os.ReadFile(filepath.Join(storageDir, "github", "acme", "tool", "latest.json"))
	if err != nil {
		t.Fatalf("读取 latest.json 失败: %v", err)
	}
	if !bytes.Contains(latestManifest, []byte(`"tag": "v2.0.0"`)) {
		t.Fatalf("latest.json 内容不符合预期: %s", string(latestManifest))
	}

	tasksRec := performRequest(router, http.MethodGet, "/api/tasks", nil)
	if tasksRec.Code != http.StatusOK {
		t.Fatalf("查询任务列表失败: %d %s", tasksRec.Code, tasksRec.Body.String())
	}
	if !bytes.Contains(tasksRec.Body.Bytes(), []byte("sync_release")) || !bytes.Contains(tasksRec.Body.Bytes(), []byte("download_asset")) {
		t.Fatalf("任务列表缺少同步/下载任务: %s", tasksRec.Body.String())
	}

	redownloadRec := performRequest(router, http.MethodPost, "/api/assets/"+strconv.Itoa(int(downloadedAssetID))+"/redownload", nil)
	if redownloadRec.Code != http.StatusOK {
		t.Fatalf("重新下载资产失败: %d %s", redownloadRec.Code, redownloadRec.Body.String())
	}
	if atomic.LoadInt32(&downloadCount) != 2 {
		t.Fatalf("重新下载后下载次数应为 2，实际 %d", downloadCount)
	}

	deleteRec := performRequest(router, http.MethodDelete, "/api/assets/"+strconv.Itoa(int(downloadedAssetID)), nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("删除资产失败: %d %s", deleteRec.Code, deleteRec.Body.String())
	}
	deletedFileRec := performRequest(router, http.MethodGet, "/api/assets/"+strconv.Itoa(int(downloadedAssetID))+"/file", nil)
	if deletedFileRec.Code != http.StatusNotFound {
		t.Fatalf("删除后读取资产文件应返回 404，实际 %d %s", deletedFileRec.Code, deletedFileRec.Body.String())
	}
}

func newTestRouter(t *testing.T) http.Handler {
	return newTestRouterWithGitHubBaseURL(t, "")
}

func newTestRouterWithGitHubBaseURL(t *testing.T, githubBaseURL string) http.Handler {
	return newTestRouterWithGitHubBaseURLAndStorageDir(t, githubBaseURL, filepath.Join(t.TempDir(), "releases"))
}

func newTestRouterWithGitHubBaseURLAndStorageDir(t *testing.T, githubBaseURL string, storageDir string) http.Handler {
	t.Helper()

	// 使用统一的全内存测试数据库（见 testhelpers_test.go），避免文件库在 t.TempDir 清理后
	// 被泄漏的后台 syncer worker 写入触发 "attempt to write a readonly database"。
	db := newTestDB(t)
	// 创建默认本地存储记录，确保 GetRepositoryStorages 能找到存储目标
	if err := database.SeedDefaultStorage(db, storageDir); err != nil {
		t.Fatalf("初始化默认存储失败: %v", err)
	}
	_ = database.BackfillAssetStorageID(db)

	return NewRouter(Dependencies{
		Config: &config.Config{
			App:     config.AppConfig{Env: "test"},
			GitHub:  config.GitHubConfig{APIBaseURL: githubBaseURL},
			Storage: config.StorageConfig{DataDir: storageDir},
		},
		DB:     db,
		Logger: zap.NewNop(),
	})
}

// newTestRouterWithStoppedSyncer 构造一个注入了“已停止 syncer”的测试路由。
// 仅用于不依赖后台异步同步的测试：checkLatest 仍会入队 sync_latest 任务，
// 但 worker 已全部退出，任务入队后无人消费，避免后台下载与测试随后的手动下载
// 在 assets 表 (release_id, name, storage_id) 唯一索引上竞态导致偶发失败。
func newTestRouterWithStoppedSyncer(t *testing.T, githubBaseURL string, storageDir string) http.Handler {
	t.Helper()

	db := newTestDB(t)
	if err := database.SeedDefaultStorage(db, storageDir); err != nil {
		t.Fatalf("初始化默认存储失败: %v", err)
	}
	_ = database.BackfillAssetStorageID(db)

	// syncer worker 使用 sync.Once 懒启动，Stop() 仅在 workersStop != nil 时关闭；
	// 因此先 UpdateMaxConcurrentTasks 触发懒启动初始化 workersStop/taskQueue，
	// 再 Stop() 关闭 workersStop 让全部 worker 退出，此后 Enqueue 仅入队不消费。
	stoppedSyncer, err := syncersvc.NewService(db, releasesvc.NewCheckService(db, nil), config.StorageConfig{DataDir: storageDir})
	if err != nil {
		t.Fatalf("构造测试 syncer 失败: %v", err)
	}
	stoppedSyncer.UpdateMaxConcurrentTasks(1)
	stoppedSyncer.Stop()

	return NewRouter(Dependencies{
		Config: &config.Config{
			App:     config.AppConfig{Env: "test"},
			GitHub:  config.GitHubConfig{APIBaseURL: githubBaseURL},
			Storage: config.StorageConfig{DataDir: storageDir},
		},
		DB:            db,
		Logger:        zap.NewNop(),
		SyncerService: stoppedSyncer,
	})
}

func performRequest(handler http.Handler, method string, path string, body []byte) *httptest.ResponseRecorder {
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		requestBody = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func TestConcurrentSyncWithMultipleAssets(t *testing.T) {
	t.Parallel()

	assetContents := map[string][]byte{
		"tool_linux_amd64.tar.gz":  []byte("linux-amd64-payload"),
		"tool_linux_arm64.tar.gz":  []byte("linux-arm64-payload"),
		"tool_darwin_amd64.tar.gz": []byte("darwin-amd64-payload"),
	}
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentMap := map[string][]byte{
			"/linux-amd64":  assetContents["tool_linux_amd64.tar.gz"],
			"/linux-arm64":  assetContents["tool_linux_arm64.tar.gz"],
			"/darwin-amd64": assetContents["tool_darwin_amd64.tar.gz"],
		}
		if content, ok := contentMap[r.URL.Path]; ok {
			_, _ = w.Write(content)
			return
		}
		http.NotFound(w, r)
	}))
	defer downloadServer.Close()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 5001,
			"tag_name": "v3.0.0",
			"name": "Release v3.0.0",
			"body": "",
			"html_url": "https://github.com/acme/tool/releases/tag/v3.0.0",
			"url": "https://api.github.com/repos/acme/tool/releases/5001",
			"published_at": "2026-06-03T10:00:00Z",
			"assets": [
				{
					"id": 6001,
					"name": "tool_linux_amd64.tar.gz",
					"size": 17,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/linux-amd64",
					"browser_download_url": "` + downloadServer.URL + `/linux-amd64"
				},
				{
					"id": 6002,
					"name": "tool_linux_arm64.tar.gz",
					"size": 17,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/linux-arm64",
					"browser_download_url": "` + downloadServer.URL + `/linux-arm64"
				},
				{
					"id": 6003,
					"name": "tool_darwin_amd64.tar.gz",
					"size": 18,
					"content_type": "application/gzip",
					"url": "` + downloadServer.URL + `/darwin-amd64",
					"browser_download_url": "` + downloadServer.URL + `/darwin-amd64"
				}
			]
		}`))
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	router := newTestRouterWithGitHubBaseURLAndStorageDir(t, githubServer.URL, storageDir)
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "tool",
		"filterMode": "regex",
		"assetIncludePatterns": ".*(linux|darwin).*amd64.*"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}

	syncRec := performRequest(router, http.MethodPost, "/api/repositories/1/sync", nil)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("同步请求失败: %d %s", syncRec.Code, syncRec.Body.String())
	}

	// 异步同步：等待后台 goroutine 完成
	var syncTaskBody struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(syncRec.Body.Bytes(), &syncTaskBody); err != nil {
		t.Fatalf("解析同步任务响应失败: %v", err)
	}
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		taskRec := performRequest(router, http.MethodGet, fmt.Sprintf("/api/tasks/%d", syncTaskBody.ID), nil)
		if taskRec.Code == http.StatusOK {
			var task struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(taskRec.Body.Bytes(), &task) == nil && (task.Status == "succeeded" || task.Status == "failed") {
				break
			}
		}
	}

	// 检查资产下载结果
	releasesRec := performRequest(router, http.MethodGet, "/api/repositories/1/releases", nil)
	if releasesRec.Code != http.StatusOK {
		t.Fatalf("查询 Release 列表失败: %d", releasesRec.Code)
	}
	var releasesBody struct {
		Items []struct {
			ID  uint   `json:"id"`
			Tag string `json:"tag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(releasesRec.Body.Bytes(), &releasesBody); err != nil {
		t.Fatalf("解析 Release 列表失败: %v", err)
	}
	if len(releasesBody.Items) == 0 {
		t.Fatalf("期望至少 1 个 Release")
	}

	assetsRec := performRequest(router, http.MethodGet, fmt.Sprintf("/api/releases/%d/assets", releasesBody.Items[0].ID), nil)
	if assetsRec.Code != http.StatusOK {
		t.Fatalf("查询资产列表失败: %d", assetsRec.Code)
	}
	var assetsBody struct {
		Items []struct {
			ID          uint   `json:"id"`
			Name        string `json:"name"`
			Status      string `json:"status"`
			StoragePath string `json:"storagePath"`
			SHA256      string `json:"sha256"`
		} `json:"items"`
	}
	if err := json.Unmarshal(assetsRec.Body.Bytes(), &assetsBody); err != nil {
		t.Fatalf("解析资产列表失败: %v", err)
	}

	verifiedCount := 0
	for _, asset := range assetsBody.Items {
		if asset.Status == "verified" {
			verifiedCount++
			if asset.StoragePath == "" || asset.SHA256 == "" {
				t.Fatalf("已验证资产 %s 缺少 storagePath 或 sha256", asset.Name)
			}
			fileRec := performRequest(router, http.MethodGet, "/api/assets/"+strconv.Itoa(int(asset.ID))+"/file", nil)
			if fileRec.Code != http.StatusOK {
				t.Fatalf("读取资产文件 %s 失败: %d", asset.Name, fileRec.Code)
			}
			expectedContent := assetContents[asset.Name]
			if !bytes.Equal(fileRec.Body.Bytes(), expectedContent) {
				t.Fatalf("资产 %s 内容不符合预期", asset.Name)
			}
		}
	}
	if verifiedCount != 2 {
		t.Fatalf("期望 2 个已验证资产（amd64 匹配），实际 %d", verifiedCount)
	}

	tasksRec := performRequest(router, http.MethodGet, "/api/tasks", nil)
	if tasksRec.Code != http.StatusOK {
		t.Fatalf("查询任务列表失败: %d", tasksRec.Code)
	}
	if !bytes.Contains(tasksRec.Body.Bytes(), []byte("download_asset")) {
		t.Fatalf("任务列表缺少 download_asset")
	}

	filesRec := performRequest(router, http.MethodGet, "/api/files", nil)
	if filesRec.Code != http.StatusOK {
		t.Fatalf("查询文件列表失败: %d", filesRec.Code)
	}
	for name, content := range assetContents {
		_ = content
		if name == "tool_linux_arm64.tar.gz" {
			continue
		}
		if !bytes.Contains(filesRec.Body.Bytes(), []byte(name)) {
			t.Fatalf("文件列表缺少 %s", name)
		}
	}
}

func TestCheckAllReleasesPersistsMultipleReleases(t *testing.T) {
	t.Parallel()

	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 只响应 releases 列表请求（不含 /latest）
		if r.URL.Path == "/repos/acme/tool/releases" {
			_, _ = w.Write([]byte(`[
				{
					"id": 7001,
					"tag_name": "v3.0.0",
					"name": "Release v3.0.0",
					"body": "major release",
					"html_url": "https://github.com/acme/tool/releases/tag/v3.0.0",
					"url": "https://api.github.com/repos/acme/tool/releases/7001",
					"published_at": "2026-06-01T10:00:00Z",
					"assets": [
						{
							"id": 8001,
							"name": "tool_linux_amd64.tar.gz",
							"size": 20,
							"content_type": "application/gzip",
							"url": "https://api.github.com/repos/acme/tool/releases/assets/8001",
							"browser_download_url": "https://github.com/acme/tool/releases/download/v3.0.0/tool_linux_amd64.tar.gz"
						}
					]
				},
				{
					"id": 7002,
					"tag_name": "v2.1.0",
					"name": "Release v2.1.0",
					"body": "minor release",
					"html_url": "https://github.com/acme/tool/releases/tag/v2.1.0",
					"url": "https://api.github.com/repos/acme/tool/releases/7002",
					"published_at": "2026-05-15T10:00:00Z",
					"assets": [
						{
							"id": 8002,
							"name": "tool_linux_amd64.tar.gz",
							"size": 18,
							"content_type": "application/gzip",
							"url": "https://api.github.com/repos/acme/tool/releases/assets/8002",
							"browser_download_url": "https://github.com/acme/tool/releases/download/v2.1.0/tool_linux_amd64.tar.gz"
						},
						{
							"id": 8003,
							"name": "tool_darwin_amd64.tar.gz",
							"size": 19,
							"content_type": "application/gzip",
							"url": "https://api.github.com/repos/acme/tool/releases/assets/8003",
							"browser_download_url": "https://github.com/acme/tool/releases/download/v2.1.0/tool_darwin_amd64.tar.gz"
						}
					]
				}
			]`))
			return
		}

		http.NotFound(w, r)
	}))
	defer githubServer.Close()

	storageDir := filepath.Join(t.TempDir(), "releases")
	router := newTestRouterWithGitHubBaseURLAndStorageDir(t, githubServer.URL, storageDir)

	// 创建仓库
	createRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{
		"owner": "acme",
		"repo": "tool"
	}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", createRec.Code, createRec.Body.String())
	}

	// 全量检查
	checkAllRec := performRequest(router, http.MethodPost, "/api/repositories/1/check-all", nil)
	if checkAllRec.Code != http.StatusOK {
		t.Fatalf("全量检查失败: %d %s", checkAllRec.Code, checkAllRec.Body.String())
	}

	var result struct {
		Releases      int `json:"releases"`
		NewReleases   int `json:"newReleases"`
		TotalAssets   int `json:"totalAssets"`
		PendingAssets int `json:"pendingAssets"`
		SkippedAssets int `json:"skippedAssets"`
	}
	if err := json.Unmarshal(checkAllRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("解析全量检查响应失败: %v", err)
	}
	if result.Releases != 2 {
		t.Fatalf("期望 2 个 Release，实际 %d", result.Releases)
	}
	if result.NewReleases != 2 {
		t.Fatalf("期望 2 个新增 Release，实际 %d", result.NewReleases)
	}
	if result.TotalAssets != 3 {
		t.Fatalf("期望 3 个资产，实际 %d", result.TotalAssets)
	}

	// 验证 Release 列表
	releasesRec := performRequest(router, http.MethodGet, "/api/repositories/1/releases", nil)
	if releasesRec.Code != http.StatusOK {
		t.Fatalf("查询 Release 列表失败: %d", releasesRec.Code)
	}
	var releasesBody struct {
		Items []struct {
			Tag      string `json:"tag"`
			IsLatest bool   `json:"isLatest"`
		} `json:"items"`
	}
	if err := json.Unmarshal(releasesRec.Body.Bytes(), &releasesBody); err != nil {
		t.Fatalf("解析 Release 列表失败: %v", err)
	}
	if len(releasesBody.Items) != 2 {
		t.Fatalf("期望 2 个 Release 记录，实际 %d", len(releasesBody.Items))
	}
	// 第一个（最新的）应为 isLatest=true
	if !releasesBody.Items[0].IsLatest {
		t.Fatal("第一个 Release 应为 isLatest=true")
	}
	if releasesBody.Items[1].IsLatest {
		t.Fatal("第二个 Release 不应为 isLatest=true")
	}
}
