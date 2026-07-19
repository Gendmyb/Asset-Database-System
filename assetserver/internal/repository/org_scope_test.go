// Package repository — G9 行级数据权限单测
package repository

import (
	"testing"
)

// TestOrgScope_Clause_OrgMode 组织级模式 (默认/开关关闭) → org_id = $N, 与历史行为一致
func TestOrgScope_Clause_OrgMode(t *testing.T) {
	s := OrgScope{OrgID: "org-1", Role: "manager", Mode: ScopeOrg}
	clause, args := s.Clause(1)
	if clause != "org_id = $1" {
		t.Fatalf("org mode clause: want %q, got %q", "org_id = $1", clause)
	}
	if len(args) != 1 || args[0].(string) != "org-1" {
		t.Fatalf("org mode args: want [org-1], got %v", args)
	}
}

// TestOrgScope_Clause_SuperAdminGlobal 部门级 + super_admin → TRUE (全局可见, 不消耗参数)
func TestOrgScope_Clause_SuperAdminGlobal(t *testing.T) {
	s := OrgScope{OrgID: "org-1", Role: "super_admin", Mode: ScopeDepartment}
	clause, args := s.Clause(1)
	if clause != "TRUE" {
		t.Fatalf("super_admin clause: want TRUE, got %q", clause)
	}
	if args != nil {
		t.Fatalf("super_admin args: want nil, got %v", args)
	}
}

// TestOrgScope_Clause_DepartmentMode 部门级 + 非 super_admin → IN 子树子查询
func TestOrgScope_Clause_DepartmentMode(t *testing.T) {
	s := OrgScope{OrgID: "dept-A", Role: "manager", Mode: ScopeDepartment}
	clause, args := s.Clause(3)
	// 应使用 $3 作为参数, 子查询匹配本部门及子孙
	wantContains := "org_id IN (SELECT id FROM assets.organizations WHERE path <@ (SELECT path FROM assets.organizations WHERE id = $3))"
	if clause != wantContains {
		t.Fatalf("department clause: want %q, got %q", wantContains, clause)
	}
	if len(args) != 1 || args[0].(string) != "dept-A" {
		t.Fatalf("department args: want [dept-A], got %v", args)
	}
}

// TestOrgScope_Clause_DepartmentQualifed 多表 JOIN 场景需限定列名
func TestOrgScope_Clause_DepartmentQualified(t *testing.T) {
	s := OrgScope{OrgID: "dept-A", Role: "manager", Mode: ScopeDepartment}
	clause, args := s.ClauseFor("asn.org_id", 1)
	wantContains := "asn.org_id IN (SELECT id FROM assets.organizations WHERE path <@ (SELECT path FROM assets.organizations WHERE id = $1))"
	if clause != wantContains {
		t.Fatalf("qualified department clause: want %q, got %q", wantContains, clause)
	}
	if len(args) != 1 || args[0].(string) != "dept-A" {
		t.Fatalf("qualified department args: want [dept-A], got %v", args)
	}
}

// TestOrgScope_Clause_DepartmentEmptyOrgID 部门级但 OrgID 空 → 回退组织级 (空串参数)
// 防御性: 调用方未设置 OrgID 时不应产生全局可见的子查询
func TestOrgScope_Clause_DepartmentEmptyOrgID(t *testing.T) {
	s := OrgScope{OrgID: "", Role: "manager", Mode: ScopeDepartment}
	clause, args := s.Clause(1)
	if clause != "org_id = $1" {
		t.Fatalf("empty org dept clause: want %q, got %q", "org_id = $1", clause)
	}
	if len(args) != 1 || args[0].(string) != "" {
		t.Fatalf("empty org dept args: want [\"\"], got %v", args)
	}
}

// TestOrgScope_IsGlobalVisible
func TestOrgScope_IsGlobalVisible(t *testing.T) {
	cases := []struct {
		name string
		s    OrgScope
		want bool
	}{
		{"super_admin+dept", OrgScope{Role: "super_admin", Mode: ScopeDepartment}, true},
		{"super_admin+org", OrgScope{Role: "super_admin", Mode: ScopeOrg}, false},
		{"manager+dept", OrgScope{Role: "manager", Mode: ScopeDepartment}, false},
		{"manager+org", OrgScope{Role: "manager", Mode: ScopeOrg}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.IsGlobalVisible(); got != tc.want {
				t.Fatalf("IsGlobalVisible: want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestOrgScope_Clause_NoInjection 部门级 OrgID 含特殊字符不应进入 SQL 文本
// (OrgID 仅作为参数 $N 传递, 不拼接进片段)
func TestOrgScope_Clause_NoInjection(t *testing.T) {
	malicious := "dept'); DROP TABLE assets.assets;--"
	s := OrgScope{OrgID: malicious, Role: "manager", Mode: ScopeDepartment}
	clause, args := s.Clause(1)
	// 片段中不应出现恶意字符串
	if containsStr(clause, malicious) {
		t.Fatalf("malicious OrgID leaked into SQL fragment: %q", clause)
	}
	if len(args) != 1 || args[0].(string) != malicious {
		t.Fatalf("args should carry malicious string as parameter, got %v", args)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
