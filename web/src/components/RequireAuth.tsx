// RequireAuth — 路由守卫 (24条路由保护)
// 对应架构文档 §7 前端路由守卫
import { Navigate } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'

export default function RequireAuth({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }

  return <>{children}</>
}
