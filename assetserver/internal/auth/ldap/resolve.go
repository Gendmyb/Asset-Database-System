// Package ldap — 组成员角色解析 (Wave 3 T2)
//
// resolveRoleForGroups 根据用户所属安全组列表 (memberOf) 和启用的组映射,
// 确定最终角色与数据范围:
//   - 多组命中时取最高角色 (super_admin > admin > manager > viewer)
//   - 无命中组时回退到默认映射 (viewer + inherit, 与历史行为一致)
//   - 禁用的组忽略 (sync_enabled=false 不参与解析)
//   - 同级别角色命中多组时, data_scope 取 'self' 优先 (最严格)
package ldap

import (
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
)

// roleRank 角色优先级 (数值越大权限越高)
var roleRank = map[string]int{
	"viewer":      0,
	"manager":     1,
	"admin":       2,
	"super_admin": 3,
}

// ResolvedRole 解析结果
type ResolvedRole struct {
	Role      string // 最终角色
	DataScope string // 数据范围: 'inherit' | 'self'
}

// resolveRoleForGroups 根据用户的 memberOf 列表和启用的组映射, 解析角色与数据范围。
//
// 规则:
//  1. 对 memberOf 中的每个 DN, 查找 mappings 中匹配的 GroupDN
//  2. 忽略 sync_enabled=false 的组 (传入的 mappings 应已过滤)
//  3. 多组命中取最高角色
//  4. 同级别角色中, data_scope='self' 优先 (更严格)
//  5. 无命中 → 默认 viewer + inherit
func resolveRoleForGroups(memberOf []string, mappings []repository.GroupMapping) (role, scope string) {
	// 构建 DN → mapping 的快速查找表
	byDN := make(map[string]repository.GroupMapping, len(mappings))
	for _, m := range mappings {
		byDN[m.GroupDN] = m
	}

	bestRole := "viewer"
	bestScope := "inherit"
	bestRank := -1

	for _, dn := range memberOf {
		m, ok := byDN[dn]
		if !ok {
			continue
		}
		if !m.SyncEnabled {
			continue
		}
		rank, exists := roleRank[m.Role]
		if !exists {
			continue
		}
		if rank > bestRank {
			bestRank = rank
			bestRole = m.Role
			bestScope = m.DataScope
		} else if rank == bestRank && m.DataScope == "self" {
			// 同级别下 self 更严格, 优先采纳
			bestScope = "self"
		}
	}

	if bestRank < 0 {
		// 无命中: 默认 viewer + inherit (与历史行为一致, 最小权限)
		return "viewer", "inherit"
	}
	return bestRole, bestScope
}

// ResolveRoleForGroups 公开包装 (供 sync.go 和 auth.go 调用)
func ResolveRoleForGroups(memberOf []string, mappings []repository.GroupMapping) ResolvedRole {
	r, s := resolveRoleForGroups(memberOf, mappings)
	return ResolvedRole{Role: r, DataScope: s}
}
