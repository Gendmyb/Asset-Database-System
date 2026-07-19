// 角色权限辅助 — 与后端 middleware/rbac.go 的 roleLevel 保持一致
// viewer=0, manager=1, admin=2, super_admin=3
import { useAuthStore } from '../store/authStore'

const LEVEL: Record<string, number> = {
  viewer: 0,
  manager: 1,
  admin: 2,
  super_admin: 3,
}

export function roleLevel(role?: string): number {
  return LEVEL[role || 'viewer'] ?? 0
}

// manager+ : 可写操作 (资产 CRUD/领用/报修/工单创建等)
export function canManage(role?: string): boolean {
  return roleLevel(role) >= LEVEL.manager
}

// admin+ : 管理操作 (报废/盘点创建/用户管理/webhook 等)
export function canAdmin(role?: string): boolean {
  return roleLevel(role) >= LEVEL.admin
}

// 组合 hook, 组件内直接解构
export function useRole() {
  const role = useAuthStore((s) => s.user?.role)
  return { role, canManage: canManage(role), canAdmin: canAdmin(role) }
}
