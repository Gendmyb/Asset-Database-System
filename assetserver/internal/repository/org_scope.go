// Package repository — 行级数据权限 (G9 部门级可见范围 + Wave 3 T5 个人数据范围)
//
// 三种模式:
//   1. ScopeOrg 组织级 (默认, 历史行为): org_id = $org
//   2. ScopeDepartment 部门级 (G9): 本部门及子孙部门; super_admin 全局可见
//   3. ScopeSelf 个人数据范围 (T5): 仅见分配给自己的资产 (active assignments)
//
// 模式优先级: ScopeSelf > ScopeDepartment > ScopeOrg
// super_admin 在 ScopeSelf 模式下也不受限制 (管理需求可审计个人资产)
package repository

import "fmt"

// ScopeMode 行级可见范围模式
type ScopeMode int

const (
	// ScopeOrg 组织级 (默认, 历史行为): org_id = $org
	ScopeOrg ScopeMode = iota
	// ScopeDepartment 部门级: 本部门及子孙部门; super_admin 全局可见
	ScopeDepartment
	// ScopeSelf 个人数据范围: 仅见分配给自己的资产 (T5)
	ScopeSelf
)

// OrgScope 行级可见范围上下文 (从 JWT claims + 配置开关推导)
//
// 零值 (Mode=ScopeOrg, OrgID="", UserID="") 会生成 org_id = $N 且参数为空串,
// 调用方必须设置 OrgID。
type OrgScope struct {
	OrgID  string
	Role   string
	Mode   ScopeMode
	UserID string // ScopeSelf 模式下用于过滤 assigned_to (T5)
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
// 四种情况:
//  1. ScopeDepartment + super_admin → "TRUE" (全局可见, 无过滤)
//  2. ScopeSelf → "<col> IN (SELECT asset_id FROM assets.assignments WHERE assigned_to=$uid AND status='active')"
//     或直接用于 asset 表的 assigned_to 子查询 (T5)
//  3. ScopeDepartment + 其他 → "<col> IN (SELECT id FROM organizations WHERE path <@ (SELECT path FROM organizations WHERE id = $N))"
//  4. ScopeOrg (默认/开关关闭) → "<col> = $N" (历史行为)
func (s OrgScope) ClauseFor(col string, nextIdx int) (string, []interface{}) {
	// 1. ScopeSelf: 个人数据范围 (T5)
	//    super_admin 在 self 模式下也受限 (self 模式是用户级设置, 按需)
	if s.Mode == ScopeSelf && s.UserID != "" {
		return fmt.Sprintf(
			"%s IN (SELECT asset_id FROM assets.assignments WHERE assigned_to = $%d AND status = 'active')",
			col, nextIdx,
		), []interface{}{s.UserID}
	}

	// 2. ScopeDepartment + super_admin → 全局 (G9)
	if s.Mode == ScopeDepartment && s.Role == "super_admin" {
		return "TRUE", nil
	}

	// 3. ScopeDepartment + 其他 → 部门子树 (G9)
	if s.Mode == ScopeDepartment && s.OrgID != "" {
		return fmt.Sprintf(
			"%s IN (SELECT id FROM assets.organizations WHERE path <@ (SELECT path FROM assets.organizations WHERE id = $%d))",
			col, nextIdx,
		), []interface{}{s.OrgID}
	}

	// 4. 默认: 组织级过滤 (与历史行为一致)
	return fmt.Sprintf("%s = $%d", col, nextIdx), []interface{}{s.OrgID}
}

// IsGlobalVisible 是否全局可见 (无需 org 过滤)
func (s OrgScope) IsGlobalVisible() bool {
	return s.Mode == ScopeDepartment && s.Role == "super_admin"
}

// IsSelfScope 是否为个人数据范围模式 (T5)
func (s OrgScope) IsSelfScope() bool {
	return s.Mode == ScopeSelf
}
