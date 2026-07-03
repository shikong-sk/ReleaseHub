package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"releasehub/backend/internal/config"

	"go.uber.org/zap"
)

func newTokenTestRouter(t *testing.T) http.Handler {
	t.Helper()

	db := newTestDB(t)

	return NewRouter(Dependencies{
		Config: &config.Config{
			App:     config.AppConfig{Env: "test"},
			GitHub:  config.GitHubConfig{APIBaseURL: ""},
			Storage: config.StorageConfig{DataDir: t.TempDir()},
		},
		DB:     db,
		Logger: zap.NewNop(),
	})
}

func TestTokenCRUD(t *testing.T) {
	t.Parallel()

	router := newTokenTestRouter(t)

	// 创建 Token
	createRec := performRequest(router, http.MethodPost, "/api/tokens", []byte(`{"name":"test-token","token":"ghp_abcdef1234567890"}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("期望创建状态码 201，实际 %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		TokenHint string `json:"tokenHint"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("解析创建响应失败: %v", err)
	}
	if created.ID == 0 || created.Name != "test-token" {
		t.Fatalf("创建响应不符合预期: %+v", created)
	}
	if created.TokenHint == "" || created.TokenHint == "ghp_abcdef1234567890" {
		t.Fatalf("Token 值不应完整暴露: %s", created.TokenHint)
	}

	// 列表
	listRec := performRequest(router, http.MethodGet, "/api/tokens", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("期望列表状态码 200，实际 %d", listRec.Code)
	}

	// 删除（无关联仓库）
	deleteRec := performRequest(router, http.MethodDelete, "/api/tokens/1", nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("期望删除状态码 204，实际 %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestTokenDeleteBlockedByRepository(t *testing.T) {
	t.Parallel()

	router := newTokenTestRouter(t)

	// 创建 Token
	createRec := performRequest(router, http.MethodPost, "/api/tokens", []byte(`{"name":"in-use-token","token":"ghp_1234567890abcdef"}`))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("创建 Token 失败: %d %s", createRec.Code, createRec.Body.String())
	}

	// 创建引用此 Token 的仓库
	repoRec := performRequest(router, http.MethodPost, "/api/repositories", []byte(`{"owner":"hashicorp","repo":"terraform","githubTokenId":1}`))
	if repoRec.Code != http.StatusCreated {
		t.Fatalf("创建仓库失败: %d %s", repoRec.Code, repoRec.Body.String())
	}

	// 尝试删除被引用的 Token — 应该被拒绝
	deleteRec := performRequest(router, http.MethodDelete, "/api/tokens/1", nil)
	if deleteRec.Code != http.StatusConflict {
		t.Fatalf("期望删除被引用 Token 返回 409，实际 %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(deleteRec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("解析错误响应失败: %v", err)
	}
	if errBody.Error == "" {
		t.Fatal("错误信息不应为空")
	}
}

func TestTokenDeleteNonExistent(t *testing.T) {
	t.Parallel()

	router := newTokenTestRouter(t)

	deleteRec := performRequest(router, http.MethodDelete, "/api/tokens/999", nil)
	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("期望删除不存在的 Token 返回 404，实际 %d", deleteRec.Code)
	}
}
