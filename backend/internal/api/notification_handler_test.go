package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestNotificationCRUD(t *testing.T) {
	db := setupNotificationTestDB(t)
	router := gin.New()
	registerNotificationRoutes(router, db)

	// 创建通知
	createBody := `{"name":"Gotify 通知","type":"gotify","serverUrl":"https://gotify.test.com","token":"test-token-123","enabled":true,"events":"*"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/notifications", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("创建通知失败: %d %s", createW.Code, createW.Body.String())
	}

	var createResp struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		ServerURL string `json:"serverUrl"`
		TokenHint string `json:"tokenHint"`
		Enabled   bool   `json:"enabled"`
		Events    string `json:"events"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if createResp.Name != "Gotify 通知" {
		t.Errorf("期望 name=Gotify 通知, 实际=%s", createResp.Name)
	}
	if createResp.TokenHint == "" {
		t.Error("期望 tokenHint 不为空")
	}

	// 列表
	listReq := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	if listW.Code != http.StatusOK {
		t.Fatalf("列表通知失败: %d", listW.Code)
	}

	var listResp struct {
		Items []models.Notification `json:"items"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("解析列表响应失败: %v", err)
	}
	if len(listResp.Items) != 1 {
		t.Errorf("期望 1 个通知, 实际=%d", len(listResp.Items))
	}

	// 更新
	updateBody := `{"name":"更新通知","type":"webhook","serverUrl":"https://webhook.test.com/hook","token":"new-token","enabled":false,"events":"new_release,sync_failed"}`
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/notifications/1", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	router.ServeHTTP(updateW, updateReq)

	if updateW.Code != http.StatusOK {
		t.Fatalf("更新通知失败: %d %s", updateW.Code, updateW.Body.String())
	}

	var updateResp struct {
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
		Events  string `json:"events"`
	}
	if err := json.Unmarshal(updateW.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("解析更新响应失败: %v", err)
	}
	if updateResp.Type != "webhook" {
		t.Errorf("期望 type=webhook, 实际=%s", updateResp.Type)
	}
	if updateResp.Enabled {
		t.Error("期望 enabled=false")
	}

	// 删除
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/notifications/1", nil)
	deleteW := httptest.NewRecorder()
	router.ServeHTTP(deleteW, deleteReq)

	if deleteW.Code != http.StatusNoContent {
		t.Fatalf("删除通知失败: %d", deleteW.Code)
	}
}
