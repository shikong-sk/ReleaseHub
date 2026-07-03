package middleware

import (
	"net/http"
	"strings"

	auditlog "releasehub/backend/internal/services/auditlog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditMiddleware 自动记录写操作（POST/PUT/PATCH/DELETE）的审计日志
// 读取操作（GET/HEAD）和认证失败/权限拒绝的请求不记录，避免日志噪音
func AuditMiddleware(db *gorm.DB) gin.HandlerFunc {
	svc := auditlog.NewService(db)
	return func(c *gin.Context) {
		// 读取操作不记录审计日志
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		c.Next()

		// 认证/鉴权被 abort 的请求（未通过中间件）不记录，避免噪音
		if c.IsAborted() {
			return
		}

		// /api/auth 下的操作由 auth handler 自己记录精确的登录/登出日志，中间件跳过避免重复
		if strings.HasPrefix(c.FullPath(), "/api/auth/") {
			return
		}

		// 仅记录 2xx 成功的写操作
		status := c.Writer.Status()
		if status < 200 || status >= 300 {
			return
		}

		actor := resolveActor(c)
		action := actionForRequest(method, c.FullPath())
		if action == "" {
			return
		}
		resource := auditResourceForPath(c.FullPath())
		if resource == "" {
			resource = "api"
		}
		detail := action + " " + resource

		// 同步记录审计日志；失败忽略，不影响主流程
		_ = svc.Record(c.Request.Context(), actor, action, resource, detail, "success", c.ClientIP())
	}
}

// resolveActor 从上下文获取操作者用户名或标识
func resolveActor(c *gin.Context) string {
	if username, exists := c.Get("username"); exists {
		if name, ok := username.(string); ok && name != "" {
			return name
		}
	}
	if authType, exists := c.Get("authType"); exists {
		if authType == "apikey" {
			return "apikey"
		}
	}
	return "anonymous"
}

// actionForRequest 根据方法和路径推断操作类型
func actionForRequest(method, path string) string {
	// 先按路径最后一段匹配特殊操作（check/sync/test 等）
	if action, ok := actionBySegment[lastPathSegment(path)]; ok {
		return action
	}
	// 按方法推断通用 CRUD 操作
	switch strings.ToUpper(method) {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return ""
	}
}

// lastPathSegment 提取路径的最后一段，如 "/api/repositories/:id/check" → "check"
func lastPathSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// actionBySegment 路径最后一段到操作类型的映射
var actionBySegment = map[string]string{
	"check":      "check",
	"check-all":  "check",
	"sync":       "sync",
	"sync-tag":   "sync",
	"cleanup":    "cleanup",
	"test":       "test",
	"pin":        "pin",
	"unpin":      "unpin",
	"download":   "download",
	"redownload": "download",
	"preview":    "preview",
	"restart":    "restart",
}

// auditResourceForPath 推断受影响的资源类型
func auditResourceForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/auth"):
		return "auth"
	case strings.HasPrefix(path, "/api/repositories"):
		return "repository"
	case strings.HasPrefix(path, "/api/releases"):
		return "release"
	case strings.HasPrefix(path, "/api/assets"):
		return "asset"
	case strings.HasPrefix(path, "/api/tasks"):
		return "task"
	case strings.HasPrefix(path, "/api/files"):
		return "file"
	case strings.HasPrefix(path, "/api/search"):
		return "search"
	case strings.HasPrefix(path, "/api/stats"):
		return "stats"
	case strings.HasPrefix(path, "/api/storages"):
		return "storage"
	case strings.HasPrefix(path, "/api/proxies"):
		return "proxy"
	case strings.HasPrefix(path, "/api/notifications"):
		return "notification"
	case strings.HasPrefix(path, "/api/tokens"):
		return "token"
	case strings.HasPrefix(path, "/api/apikeys"):
		return "apikey"
	case strings.HasPrefix(path, "/api/upload"):
		return "upload"
	case strings.HasPrefix(path, "/api/reconcile"):
		return "reconcile"
	case strings.HasPrefix(path, "/api/config"):
		return "config"
	case strings.HasPrefix(path, "/api/system"):
		return "system"
	case strings.HasPrefix(path, "/api/filter"):
		return "filter"
	default:
		return ""
	}
}
