package middleware

import "testing"

func TestRoleAllowed(t *testing.T) {
	tests := []struct {
		name  string
		role  string
		level permissionLevel
		want  bool
	}{
		{name: "admin all", role: "admin", level: permissionAdmin, want: true},
		{name: "operator write", role: "operator", level: permissionWrite, want: true},
		{name: "operator no admin", role: "operator", level: permissionAdmin, want: false},
		{name: "viewer read", role: "viewer", level: permissionRead, want: true},
		{name: "viewer no write", role: "viewer", level: permissionWrite, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roleAllowed(tt.role, tt.level); got != tt.want {
				t.Fatalf("roleAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScopeAllowed(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		rule  permissionRule
		want  bool
	}{
		{name: "wildcard", scope: "*", rule: permissionRule{Level: permissionAdmin, Resource: "apikey"}, want: true},
		{name: "read scope", scope: "read", rule: permissionRule{Level: permissionRead, Resource: "repo"}, want: true},
		{name: "read cannot write", scope: "read", rule: permissionRule{Level: permissionWrite, Resource: "repo"}, want: false},
		{name: "resource write", scope: "repo:write", rule: permissionRule{Level: permissionWrite, Resource: "repo"}, want: true},
		{name: "asset download alias", scope: "asset:download", rule: permissionRule{Level: permissionWrite, Resource: "asset"}, want: true},
		{name: "admin star", scope: "admin:*", rule: permissionRule{Level: permissionAdmin, Resource: "storage"}, want: true},
		{name: "trim comma", scope: "repo:read, asset:download", rule: permissionRule{Level: permissionWrite, Resource: "asset"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopeAllowed(tt.scope, tt.rule); got != tt.want {
				t.Fatalf("scopeAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
