package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type permissionLevel string

const (
	permissionRead  permissionLevel = "read"
	permissionWrite permissionLevel = "write"
	permissionAdmin permissionLevel = "admin"
)

type permissionRule struct {
	Level    permissionLevel
	Resource string
}

func AuthorizeRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		rule := classifyRequest(c)
		if authType, _ := c.Get("authType"); authType == "apikey" {
			scope, _ := c.Get("apiKeyScope")
			if !scopeAllowed(scopeString(scope), rule) {
				c.JSON(http.StatusForbidden, gin.H{"error": "API Key 权限不足"})
				c.Abort()
				return
			}
			c.Next()
			return
		}

		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证凭证"})
			c.Abort()
			return
		}
		if !roleAllowed(roleString(role), rule.Level) {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func classifyRequest(c *gin.Context) permissionRule {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	resource := resourceForPath(path)
	if adminResource(resource) {
		return permissionRule{Level: permissionAdmin, Resource: resource}
	}
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		return permissionRule{Level: permissionRead, Resource: resource}
	}
	return permissionRule{Level: permissionWrite, Resource: resource}
}

func resourceForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/repositories"):
		return "repo"
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
	default:
		return "api"
	}
}

func adminResource(resource string) bool {
	switch resource {
	case "storage", "proxy", "notification", "token", "apikey", "upload", "reconcile":
		return true
	default:
		return false
	}
}

func roleAllowed(role string, level permissionLevel) bool {
	switch role {
	case "admin":
		return true
	case "operator":
		return level == permissionRead || level == permissionWrite
	case "viewer":
		return level == permissionRead
	default:
		return false
	}
}

func scopeAllowed(scope string, rule permissionRule) bool {
	scopes := splitScope(scope)
	if containsScope(scopes, "*") || containsScope(scopes, string(rule.Level)) {
		return true
	}
	if rule.Level == permissionAdmin && (containsScope(scopes, "admin") || containsScope(scopes, "admin:*")) {
		return true
	}
	if containsScope(scopes, rule.Resource+":*") || containsScope(scopes, rule.Resource+":"+string(rule.Level)) {
		return true
	}
	if rule.Resource == "asset" && rule.Level == permissionWrite && containsScope(scopes, "asset:download") {
		return true
	}
	return false
}

func splitScope(scope string) []string {
	parts := strings.Split(scope, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func containsScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if scope == expected {
			return true
		}
	}
	return false
}

func scopeString(value any) string {
	if scope, ok := value.(string); ok {
		return scope
	}
	return ""
}

func roleString(value any) string {
	if role, ok := value.(string); ok {
		return role
	}
	return ""
}
