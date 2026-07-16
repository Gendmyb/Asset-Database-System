import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'

const navItems = [
  { path: '/dashboard', label: 'Dashboard', icon: '◧' },
  { path: '/assets', label: 'Assets', icon: '⊞' },
  { path: '/agents', label: 'Agents', icon: '⬡' },
  { path: '/admin', label: 'Admin', icon: '⚙' },
]

const s = (styles: Record<string, string | number>) => styles as React.CSSProperties

export default function Layout() {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  return (
    <div style={{ display:'flex', height:'100vh', background:'var(--bg-base)' }}>
      {/* Sidebar */}
      <aside style={{
        width:220, background:'var(--bg-panel)', borderRight:'1px solid var(--border-subtle)',
        display:'flex', flexDirection:'column', flexShrink:0
      }}>
        {/* Logo */}
        <div style={{ padding:'16px 20px', borderBottom:'1px solid var(--border-subtle)' }}>
          <div style={{ display:'flex', alignItems:'center', gap:10 }}>
            <div style={{
              width:32, height:32, background:'var(--brand)', borderRadius:8,
              display:'flex', alignItems:'center', justifyContent:'center'
            }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5">
                <path d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4"/>
              </svg>
            </div>
            <div>
              <div style={{ fontSize:14, fontWeight:600, color:'var(--text-primary)' }}>Asset DB</div>
              <div style={{ fontSize:11, color:'var(--text-quaternary)' }}>v0.1.0</div>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex:1, padding:'8px 12px' }}>
          {navItems.map(item => {
            const active = pathname.startsWith(item.path)
            return (
              <Link
                key={item.path}
                to={item.path}
                style={{
                  display:'flex', alignItems:'center', gap:10, padding:'8px 10px',
                  borderRadius:6, marginBottom:2, textDecoration:'none',
                  fontSize:13, fontWeight: active?510:400,
                  color: active?'var(--text-primary)':'var(--text-tertiary)',
                  background: active?'rgba(255,255,255,0.04)':'transparent',
                  transition:'all .1s'
                }}
                onMouseEnter={e => { if(!active) { e.currentTarget.style.background='rgba(255,255,255,0.02)'; e.currentTarget.style.color='var(--text-secondary)' }}}
                onMouseLeave={e => { if(!active) { e.currentTarget.style.background='transparent'; e.currentTarget.style.color='var(--text-tertiary)' }}}
              >
                <span style={{ fontSize:16, width:20, textAlign:'center' }}>{item.icon}</span>
                {item.label}
              </Link>
            )
          })}
        </nav>

        {/* User */}
        <div style={{ padding:'12px 16px', borderTop:'1px solid var(--border-subtle)' }}>
          <div style={{ display:'flex', alignItems:'center', gap:10 }}>
            <div style={{
              width:28, height:28, borderRadius:8,
              background:'linear-gradient(135deg, var(--brand), #7170ff)',
              display:'flex', alignItems:'center', justifyContent:'center',
              fontSize:12, fontWeight:600, color:'white'
            }}>
              {user?.username?.[0]?.toUpperCase() || 'A'}
            </div>
            <div style={{ flex:1, minWidth:0 }}>
              <div style={{ fontSize:12, fontWeight:500, color:'var(--text-secondary)', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                {user?.username}
              </div>
              <div style={{ fontSize:11, color:'var(--text-quaternary)' }}>{user?.role}</div>
            </div>
            <button
              onClick={() => { logout(); navigate('/login') }}
              style={{
                background:'none', border:'none', cursor:'pointer',
                color:'var(--text-quaternary)', padding:4, borderRadius:4
              }}
              title="Sign out"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/>
              </svg>
            </button>
          </div>
        </div>
      </aside>

      {/* Main */}
      <div style={{ flex:1, display:'flex', flexDirection:'column', minWidth:0 }}>
        <header style={{
          height:48, background:'var(--bg-panel)', borderBottom:'1px solid var(--border-subtle)',
          display:'flex', alignItems:'center', padding:'0 24px', gap:12, flexShrink:0
        }}>
          <span style={{ fontSize:13, fontWeight:500, color:'var(--text-secondary)' }}>
            {navItems.find(n => pathname.startsWith(n.path))?.label || ''}
          </span>
          <div style={{ flex:1 }} />
          <span style={{ width:6, height:6, borderRadius:'50%', background:'var(--success)' }} />
          <span style={{ fontSize:11, color:'var(--text-quaternary)' }}>Demo</span>
        </header>
        <main style={{ flex:1, overflow:'auto' }}>
          <Outlet />
        </main>
      </div>
    </div>
  )
}
