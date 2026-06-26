package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("期望状态码 200，实际 %d", rec.Code)
	}

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("期望健康状态 ok，实际 %s", body.Status)
	}
	if body.Checks["database"] != "ok" {
		t.Fatalf("期望数据库状态 ok，实际 %s", body.Checks["database"])
	}
}
