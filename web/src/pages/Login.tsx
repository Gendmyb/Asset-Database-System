import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '../store/authStore'
import api from '../api/client'

export default function Login() {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const authLogin = useAuthStore((s) => s.login)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const { data } = await api.post('/auth/login', { username, password })
      authLogin(data.access_token, data.refresh_token, data.user)
      navigate('/assets')
    } catch {
      setError('Invalid credentials')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--bg-base)', padding: 24
    }}>
      <div style={{
        width: '100%', maxWidth: 400,
        background: 'var(--bg-surface)', borderRadius: 12,
        border: '1px solid var(--border-default)', padding: 40
      }}>
        {/* Logo */}
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <div style={{
            width: 48, height: 48, background: 'var(--brand)', borderRadius: 12,
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
            marginBottom: 16
          }}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4"/>
            </svg>
          </div>
          <h1 style={{ fontSize: 20, fontWeight: 600, letterSpacing: '-0.24px', color: 'var(--text-primary)', marginBottom: 4 }}>
            Asset Database
          </h1>
          <p style={{ fontSize: 14, color: 'var(--text-tertiary)' }}>IT Asset Management</p>
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display:'block', fontSize:13, fontWeight:500, color:'var(--text-secondary)', marginBottom:6 }}>
              Username
            </label>
            <input
              type="text" value={username}
              onChange={e => setUsername(e.target.value)}
              style={{
                width:'100%', padding:'8px 12px', borderRadius:6, border:'1px solid var(--border-default)',
                background:'rgba(255,255,255,0.02)', color:'var(--text-primary)', fontSize:14,
                outline:'none', fontFamily:'inherit'
              }}
              placeholder="admin"
              required
            />
          </div>
          <div style={{ marginBottom: 24 }}>
            <label style={{ display:'block', fontSize:13, fontWeight:500, color:'var(--text-secondary)', marginBottom:6 }}>
              Password
            </label>
            <input
              type="password" value={password}
              onChange={e => setPassword(e.target.value)}
              style={{
                width:'100%', padding:'8px 12px', borderRadius:6, border:'1px solid var(--border-default)',
                background:'rgba(255,255,255,0.02)', color:'var(--text-primary)', fontSize:14,
                outline:'none', fontFamily:'inherit'
              }}
              placeholder="Any password (demo)"
              required
            />
          </div>
          {error && (
            <div style={{
              padding:'8px 12px', borderRadius:6, marginBottom:16,
              background:'rgba(239,68,68,0.1)', border:'1px solid rgba(239,68,68,0.2)',
              color:'#fca5a5', fontSize:13
            }}>{error}</div>
          )}
          <button
            type="submit" disabled={loading}
            style={{
              width:'100%', padding:'10px', borderRadius:6, border:'none',
              background: loading ? 'rgba(94,106,210,0.5)' : 'var(--brand)',
              color:'white', fontSize:14, fontWeight:500, cursor: loading ? 'default' : 'pointer',
              fontFamily:'inherit', transition:'background .15s'
            }}
            onMouseEnter={e => { if(!loading) e.currentTarget.style.background = 'var(--brand-hover)' }}
            onMouseLeave={e => { if(!loading) e.currentTarget.style.background = 'var(--brand)' }}
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>
        </form>
        <p style={{ textAlign:'center', marginTop:20, fontSize:12, color:'var(--text-quaternary)' }}>
          Demo mode — no real authentication required
        </p>
      </div>
    </div>
  )
}
