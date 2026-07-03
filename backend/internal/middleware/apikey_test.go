package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAPIKeyOrAuth_DisabledAuthSetsAdminRole 验证关闭认证模式下中间件设置 role=admin。
// 服务重启等 handler 会显式校验 admin 角色（c.Get("role")），若关闭认证时不注入 role，
// 这些 handler 会误判无权限而返回 403。修复要求关闭认证时所有请求视为管理员权限，
// 与 AuthorizeRequest 放行全部操作的语义保持一致。
//
// 关闭认证分支不触碰 db 参数，故传入 nil 即可，无需构造测试数据库。
func TestAPIKeyOrAuth_DisabledAuthSetsAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotRole string
	var roleExists bool
	r := gin.New()
	r.Use(APIKeyOrAuth(nil, func() bool { return false })) // 关闭认证
	r.GET("/probe", func(c *gin.Context) {
		role, exists := c.Get("role")
		roleExists = exists
		if exists {
			gotRole, _ = role.(string)
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	r.ServeHTTP(w, req)

	if !roleExists {
		t.Fatal("关闭认证模式下中间件未设置 role，handleRestart 等显式校验角色的 handler 将返回 403")
	}
	if gotRole != "admin" {
		t.Fatalf("关闭认证模式下 role 应为 admin，实际为 %q", gotRole)
	}
}
