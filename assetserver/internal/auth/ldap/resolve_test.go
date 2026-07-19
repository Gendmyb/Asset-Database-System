// Package ldap — 角色解析测试 (Wave 3 T2)
package ldap

import (
	"testing"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
)

func TestResolveRoleForGroups(t *testing.T) {
	// 准备组映射
	mappings := []repository.GroupMapping{
		{GroupDN: "CN=IT-Admins,OU=Groups,DC=corp,DC=local", Role: "admin", DataScope: "inherit", SyncEnabled: true},
		{GroupDN: "CN=Asset-Viewers,OU=Groups,DC=corp,DC=local", Role: "viewer", DataScope: "inherit", SyncEnabled: true},
		{GroupDN: "CN=Asset-Managers,OU=Groups,DC=corp,DC=local", Role: "manager", DataScope: "inherit", SyncEnabled: true},
		{GroupDN: "CN=Self-Scope-Users,OU=Groups,DC=corp,DC=local", Role: "viewer", DataScope: "self", SyncEnabled: true},
		{GroupDN: "CN=Disabled-Group,OU=Groups,DC=corp,DC=local", Role: "super_admin", DataScope: "inherit", SyncEnabled: false},
	}

	tests := []struct {
		name      string
		memberOf  []string
		wantRole  string
		wantScope string
	}{
		{
			name:      "no groups → default viewer+inherit",
			memberOf:  []string{},
			wantRole:  "viewer",
			wantScope: "inherit",
		},
		{
			name:      "no mapping hit → default viewer+inherit",
			memberOf:  []string{"CN=Unknown,OU=Groups,DC=corp,DC=local"},
			wantRole:  "viewer",
			wantScope: "inherit",
		},
		{
			name:      "single mapping → admin",
			memberOf:  []string{"CN=IT-Admins,OU=Groups,DC=corp,DC=local"},
			wantRole:  "admin",
			wantScope: "inherit",
		},
		{
			name: "multiple groups → highest role wins",
			memberOf: []string{
				"CN=Asset-Viewers,OU=Groups,DC=corp,DC=local",
				"CN=IT-Admins,OU=Groups,DC=corp,DC=local",
				"CN=Asset-Managers,OU=Groups,DC=corp,DC=local",
			},
			wantRole:  "admin",
			wantScope: "inherit",
		},
		{
			name:      "self scope group",
			memberOf:  []string{"CN=Self-Scope-Users,OU=Groups,DC=corp,DC=local"},
			wantRole:  "viewer",
			wantScope: "self",
		},
		{
			name: "self scope + higher role → higher role wins, scope from higher",
			memberOf: []string{
				"CN=Self-Scope-Users,OU=Groups,DC=corp,DC=local",
				"CN=IT-Admins,OU=Groups,DC=corp,DC=local",
			},
			wantRole:  "admin",
			wantScope: "inherit", // admin 组的 scope
		},
		{
			name: "disabled group → ignored (would be super_admin but disabled)",
			memberOf: []string{
				"CN=Disabled-Group,OU=Groups,DC=corp,DC=local",
				"CN=Asset-Viewers,OU=Groups,DC=corp,DC=local",
			},
			wantRole:  "viewer",
			wantScope: "inherit",
		},
		{
			name:      "nil memberOf → default",
			memberOf:  nil,
			wantRole:  "viewer",
			wantScope: "inherit",
		},
		{
			name:      "nil mappings → default",
			memberOf:  []string{"CN=Some-Group,DC=corp,DC=local"},
			wantRole:  "viewer",
			wantScope: "inherit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, scope := resolveRoleForGroups(tt.memberOf, mappings)
			if role != tt.wantRole {
				t.Errorf("role = %q, want %q", role, tt.wantRole)
			}
			if scope != tt.wantScope {
				t.Errorf("scope = %q, want %q", scope, tt.wantScope)
			}
		})
	}
}

func TestResolvedRoleExported(t *testing.T) {
	mappings := []repository.GroupMapping{
		{GroupDN: "CN=Managers,DC=corp,DC=local", Role: "manager", DataScope: "inherit", SyncEnabled: true},
	}
	memberOf := []string{"CN=Managers,DC=corp,DC=local"}
	result := ResolveRoleForGroups(memberOf, mappings)
	if result.Role != "manager" {
		t.Errorf("Role = %q, want manager", result.Role)
	}
	if result.DataScope != "inherit" {
		t.Errorf("DataScope = %q, want inherit", result.DataScope)
	}
}
