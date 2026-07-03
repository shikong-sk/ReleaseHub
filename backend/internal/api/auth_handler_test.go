package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"releasehub/backend/internal/middleware"
	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// 统一使用全内存测试数据库（见 testhelpers_test.go）
	return newTestDB(t)
}

func createTestUser(db *gorm.DB, username, password, role string) models.User {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	user := models.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		Enabled:      true,
	}
	db.Create(&user)
	return user
}

func TestAuthLogin(t *testing.T) {
	middleware.SetJWTSecret("test-secret")
	db := setupAuthTestDB(t)
	createTestUser(db, "admin", "password123", "admin")

	router := gin.New()
	registerAuthRoutes(router, db)

	// 正确登录
	loginBody := `{"username":"admin","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("登录失败: %d %s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Token == "" {
		t.Error("期望返回 Token")
	}
	if resp.User.Username != "admin" {
		t.Errorf("期望 username=admin, 实际=%s", resp.User.Username)
	}
	if resp.User.Role != "admin" {
		t.Errorf("期望 role=admin, 实际=%s", resp.User.Role)
	}

	// 错误密码
	badBody := `{"username":"admin","password":"wrong"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(badBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("期望 401, 实际=%d", w2.Code)
	}
}

func TestAuthCreateUser(t *testing.T) {
	middleware.SetJWTSecret("test-secret")
	db := setupAuthTestDB(t)
	admin := createTestUser(db, "admin", "password123", "admin")

	// 生成管理员 Token
	token, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatalf("生成 Token 失败: %v", err)
	}

	router := gin.New()
	registerAuthRoutes(router, db)

	// 创建用户
	createBody := `{"username":"viewer1","password":"pass123456","role":"viewer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("创建用户失败: %d %s", w.Code, w.Body.String())
	}

	var resp models.User
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Username != "viewer1" {
		t.Errorf("期望 username=viewer1, 实际=%s", resp.Username)
	}
	if resp.Role != "viewer" {
		t.Errorf("期望 role=viewer, 实际=%s", resp.Role)
	}
}
