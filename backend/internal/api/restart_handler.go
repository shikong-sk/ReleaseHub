package api

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// registerRestartRoutes 注册服务重启接口。
// 仅 admin 可访问（受 permission 中间件保护，POST 方法需 write 权限，
// 但为安全起见显式检查 admin 角色）。
func registerRestartRoutes(router *gin.Engine) {
	router.POST("/api/system/restart", handleRestart)
}

// handleRestart 异步触发进程优雅退出，由外部进程管理器重新拉起。
// 先返回 HTTP 响应，再延迟发送信号，确保客户端能收到响应。
func handleRestart(c *gin.Context) {
	// 校验为 admin 角色
	role, exists := c.Get("role")
	roleStr, _ := role.(string)
	if !exists || roleStr != "admin" {
		writeError(c, http.StatusForbidden, "仅管理员可重启服务")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "服务正在重启"})

	// 异步发送 SIGTERM，让响应先写回客户端
	go func() {
		time.Sleep(200 * time.Millisecond)
		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			return
		}
		_ = p.Signal(syscall.SIGTERM)
	}()
}
