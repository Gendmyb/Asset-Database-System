// Package repository — 行级数据权限 (G9 部门级可见范围)
//
// 在既有组织级 org_id 过滤基础上, 支持"部门级"可见范围:
// 非超级管理员的用户只能看本部门及下级部门 (organizations.path ltree 子树) 的数据,
// super_admin 全局可见。默认行为 (ScopeOrg) 与历史完全一致 (org_id = $org)。
package repository

import "fmt"

// ScopeMode 行级可见范围模式
type ScopeMode int

const (
	// ScopeOrg 组织级 (默认, 历史行为): org_id = $org
	ScopeOrg ScopeMode = iota
	// ScopeDepartment 部门级: 本部门及子孙部门; super_admin 全局可见
	ScopeDepartment
)

// OrgScope 行级可见范围上下文 (从 JWT claims + 配置开关推导)
//
// 零值 (Mode=ScopeOrg, OrgID="") 会生成 org_id = $N 且参数为空串,
// 调用方必须设置 OrgID。为避免遗忘, 各仓储在 OrgID 为空时回退到过滤字段自带的 OrgID。
type OrgScope struct {
	OrgID string
	Role  string
	Mode  ScopeMode
}

// Clause 返回参数化的 org_id 过滤 SQL 片段 (列名固定为 org_id)。
// 等价于 ClauseFor("org_id", nextIdx)。单表查询无歧义时使用。
func (s OrgScope) Clause(nextIdx int) (string, []interface{}) {
	return s.ClauseFor("org_id", nextIdx)
}

// ClauseFor 返回参数化的指定列过滤 SQL 片段。
// col 用于多表 JOIN 场景限定列名 (如 "asn.org_id"), 避免歧义; 列名由调用方硬编码, 不接受用户输入。
//
//	nextIdx: 该片段使用的起始位置参数下标 ($N)
//	返回: SQL 片段 (不含前导 AND), 以及需要追加的参数 (顺序对应 $nextIdx..)
//
// 三种情况:
//  1. ScopeDepartment + super_admin → "TRUE" (全局可见, 无过滤)
//  2. ScopeDepartment + 其他 → "<col> IN (SELECT id FROM organizations WHERE path <@ 本部门 path)"
//     利用 ltree <@ 操作符匹配本部门及子孙; path 由子查询在服务端取, 无注入风险。
//  3. ScopeOrg (默认/开关关闭) → "<col> = $N" (历史行为)
func (s OrgScope) ClauseFor(col string, nextIdx int) (string, []interface{}) {
	if s.Mode == ScopeDepartment && s.Role == "super_admin" {
		// 超级管理员全局可见; 返回恒真条件, 不消耗参数
		return "TRUE", nil
	}
	if s.Mode == ScopeDepartment && s.OrgID != "" {
		// 本部门及子孙部门 (ltree 子树匹配, path <@ 自身为真 → 含本部门)
		return fmt.Sprintf(
			"%s IN (SELECT id FROM assets.organizations WHERE path <@ (SELECT path FROM assets.organizations WHERE id = $%d))",
			col, nextIdx,
		), []interface{}{s.OrgID}
	}
	// 默认: 组织级过滤 (与历史行为一致)
	return fmt.Sprintf("%s = $%d", col, nextIdx), []interface{}{s.OrgID}
}

// IsGlobalVisible 是否全局可见 (无需 org 过滤)
func (s OrgScope) IsGlobalVisible() bool {
	return s.Mode == ScopeDepartment && s.Role == "super_admin"
}
