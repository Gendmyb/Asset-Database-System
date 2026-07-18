import { Link } from 'react-router-dom'

export default function NotFound() {
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
      minHeight: '60vh', padding: 32, textAlign: 'center'
    }}>
      <div style={{ fontSize: 64, fontWeight: 700, color: 'var(--text-quaternary)', marginBottom: 16 }}>404</div>
      <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', marginBottom: 8 }}>
        页面未找到
      </h1>
      <p style={{ fontSize: 14, color: 'var(--text-tertiary)', marginBottom: 24 }}>
        您访问的页面不存在或已被移除
      </p>
      <Link to="/" style={{
        padding: '10px 20px', borderRadius: 6, background: 'var(--brand)',
        color: 'white', textDecoration: 'none', fontSize: 14, fontWeight: 500
      }}>
        返回首页
      </Link>
    </div>
  )
}
