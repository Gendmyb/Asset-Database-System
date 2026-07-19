// 管理后台布局 — 顶部 Tab 导航 (用户 / 系统设置 / Webhooks)
import { NavLink, Outlet } from 'react-router-dom'

const TABS: { to: string; label: string; end?: boolean }[] = [
  { to: '/admin/users', label: '用户', end: true },
  { to: '/admin/settings', label: '系统设置' },
  { to: '/admin/webhooks', label: 'Webhooks' },
  { to: '/admin/directory', label: '目录集成' },
]

export default function AdminLayout() {
  return (
    <div>
      <div
        style={{
          display: 'flex',
          gap: 24,
          padding: '0 32px',
          borderBottom: '1px solid var(--border-subtle)',
        }}
      >
        {TABS.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            end={t.end}
            style={({ isActive }) => ({
              padding: '12px 0',
              fontSize: 13,
              fontWeight: isActive ? 600 : 400,
              color: isActive ? 'var(--text-primary)' : 'var(--text-tertiary)',
              textDecoration: 'none',
              borderBottom: isActive ? '2px solid var(--brand)' : '2px solid transparent',
              transition: 'color .1s',
            })}
          >
            {t.label}
          </NavLink>
        ))}
      </div>
      <Outlet />
    </div>
  )
}
