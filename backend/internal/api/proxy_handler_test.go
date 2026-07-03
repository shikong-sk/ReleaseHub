package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupProxyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 统一使用全内存测试数据库（见 testhelpers_test.go）
	return newTestDB(t)
}

func TestProxyCRUD(t *testing.T) {
	db := setupProxyTestDB(t)
	router := gin.New()
	registerProxyRoutes(router, db)

	// 创建代理
	createBody := `{"name":"测试代理","type":"http","host":"proxy.example.com","port":7890}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/proxies", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("创建代理失败: %d %s", createW.Code, createW.Body.String())
	}

	var createResp struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if createResp.Name != "测试代理" {
		t.Errorf("期望 name=测试代理, 实际=%s", createResp.Name)
	}
	if createResp.Port != 7890 {
		t.Errorf("期望 port=7890, 实际=%d", createResp.Port)
	}

	// 列表
	listReq := httptest.NewRequest(http.MethodGet, "/api/proxies", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("列表代理失败: %d", listW.Code)
	}

	var listResp struct {
		Items []models.Proxy `json:"items"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析列表响应失败: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Errorf("期望 1 个代理, 实际=%d", len(listResp.Items))
	}

	// 更新
	updateBody := `{"name":"更新代理","type":"socks5","host":"socks.example.com","port":1080}`
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/proxies/1", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	router.ServeHTTP(updateW, updateReq)

	if updateW.Code != http.StatusOK {
		t.Fatalf("更新代理失败: %d %s", updateW.Code, updateW.Body.String())
	}

	var updateResp models.Proxy
	if err := json.Unmarshal(updateW.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("解析更新响应失败: %v", err)
	}
	if updateResp.Type != "socks5" {
		t.Errorf("期望 type=socks5, 实际=%s", updateResp.Type)
	}

	// 删除
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/proxies/1", nil)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)

	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("删除代理失败: %d", deleteW.Code)
	}
}

func TestProxyDeleteBlockedByRepository(t *testing.T) {
	db := setupProxyTestDB(t)

	// 创建代理
	proxy := models.Proxy{Name: "被引用代理", Type: "http", Host: "proxy.test", Port: 8080}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatalf("创建代理失败: %v", err)
	}

	// 创建仓库引用该代理
	proxyID := proxy.ID
	repo := models.Repository{
		Provider: "github", Owner: "test", Repo: "repo",
		Enabled: true, ProxyID: &proxyID,
		IntervalSeconds: 1800, FilterMode: "glob", RetentionKeepLatest: 5,
		LastStatus: models.RepositoryStatusUnknown,
	}
	if err := db.Create(&repo).Error; err != nil {
		t.Fatalf("创建仓库失败: %v", err)
	}

	router := gin.New()
	registerProxyRoutes(router, db)

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/proxies/1", nil)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)

	if deleteW.Code != http.StatusConflict {
		t.Errorf("期望 409 冲突, 实际=%d", deleteW.Code)
	}
}
