import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import { useEffect, useState } from 'react'

interface NavItem {
  path: string
  label: string
  icon: string
  demoOnly?: boolean
  adminOnly?: boolean
}

const navItems: NavItem[] = [
  { path: '/dashboard', label: '仪表盘', icon: '◧' },
  { path: '/assets', label: '资产管理', icon: '⊞' },
  { path: '/assignments', label: '领用与借用', icon: '⇄' },
  { path: '/maintenance', label: '维修保养', icon: '⚕', demoOnly: true },
  { path: '/stocktakes', label: '盘点', icon: '◎', demoOnly: true },
  { path: '/reports', label: '报表', icon: '▤', demoOnly: true },
  { path: '/admin', label: '管理', icon: '⚙', adminOnly: true },
]

export default function Layout() {
  const { user, logout } = useAuthStore()
  const refreshToken = useAuthStore((s) => s.refreshToken)
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [demoMode, setDemoMode] = useState(false)
  const isAdmin = user?.role === 'admin' || user?.role === 'super_admin'

  useEffect(() => {
    fetch('/healthz')
      .then((res) => res.json())
      .then((data) => {
        if (data.mode === 'demo') setDemoMode(true)
      })
      .catch(() => {})
  }, [])

  const filteredNav = navItems.filter((item) => {
    if (item.adminOnly && !isAdmin) return false
    if (item.demoOnly && !demoMode) return false
    return true
  })

  return (
    <div style={{ display: 'flex', height: '100vh', background: 'var(--bg-base)' }}>
      {/* Sidebar */}
      <aside
        style={{
          width: 220,
          background: 'var(--bg-panel)',
          borderRight: '1px solid var(--border-subtle)',
          display: 'flex',
          flexDirection: 'column',
          flexShrink: 0,
        }}
      >
        {/* Logo */}
        <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border-subtle)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div
              style={{
                width: 32,
                height: 32,
                background: 'var(--brand)',
                borderRadius: 8,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5">
                <path d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
              </svg>
            </div>
            <div>
              <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>Asset DB</div>
              <div style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>v0.1.0</div>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex: 1, padding: '8px 12px' }}>
          {filteredNav.map((item) => {
            const active = pathname.startsWith(item.path)
            return (
              <Link
                key={item.path}
                to={item.path}
                title={item.demoOnly && demoMode ? `${item.label}（生产模式可用）` : item.label}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  padding: '8px 10px',
                  borderRadius: 6,
                  marginBottom: 2,
                  textDecoration: 'none',
                  fontSize: 13,
                  fontWeight: active ? 510 : 400,
                  color: active ? 'var(--text-primary)' : 'var(--text-tertiary)',
                  background: active ? 'rgba(255,255,255,0.04)' : 'transparent',
                  transition: 'all .1s',
                  opacity: item.demoOnly && !demoMode ? 0.4 : 1,
                }}
                onMouseEnter={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'rgba(255,255,255,0.02)'
                    e.currentTarget.style.color = 'var(--text-secondary)'
                  }
                }}
                onMouseLeave={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'transparent'
                    e.currentTarget.style.color = 'var(--text-tertiary)'
                  }
                }}
              >
                <span style={{ fontSize: 16, width: 20, textAlign: 'center' }}>{item.icon}</span>
                {item.label}
                {item.demoOnly && !demoMode && (
                  <span style={{ fontSize: 10, color: 'var(--text-quaternary)', marginLeft: 'auto' }}>PRO</span>
                )}
              </Link>
            )
          })}
        </nav>

        {/* User */}
        <div style={{ padding: '12px 16px', borderTop: '1px solid var(--border-subtle)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div
              style={{
                width: 28,
                height: 28,
                borderRadius: 8,
                background: 'linear-gradient(135deg, var(--brand), #7170ff)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 12,
                fontWeight: 600,
                color: 'white',
              }}
            >
              {user?.username?.[0]?.toUpperCase() || 'A'}
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div
                style={{
                  fontSize: 12,
                  fontWeight: 500,
                  color: 'var(--text-secondary)',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {user?.username}
              </div>
              <div style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>{user?.role}</div>
            </div>
            <button
              onClick={() => {
                if (refreshToken) {
                  import('../api/auth')
                    .then((authApi) =>
                      authApi.logout(refreshToken).catch(() => undefined)
                    )
                    .finally(() => {
                      logout()
                      navigate('/login')
                    })
                } else {
                  logout()
                  navigate('/login')
                }
              }}
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                color: 'var(--text-quaternary)',
                padding: 4,
                borderRadius: 4,
              }}
              title="退出登录"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
              </svg>
            </button>
          </div>
        </div>
      </aside>

      {/* Main */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <header
          style={{
            height: 48,
            background: 'var(--bg-panel)',
            borderBottom: '1px solid var(--border-subtle)',
            display: 'flex',
            alignItems: 'center',
            padding: '0 24px',
            gap: 12,
            flexShrink: 0,
          }}
        >
          <span style={{ fontSize: 13, fontWeight: 500, color: 'var(--text-secondary)' }}>
            {navItems.find((n) => pathname.startsWith(n.path))?.label || ''}
          </span>
          <div style={{ flex: 1 }} />
          {demoMode && (
            <>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--success)' }} />
              <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>演示模式</span>
            </>
          )}
        </header>
        <main style={{ flex: 1, overflow: 'auto' }}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
