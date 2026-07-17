// Agents — Agent status page with online/offline indicators + last heartbeat
import { useEffect, useState } from 'react'
import api from '../api/client'
import type { Agent, AgentHealthStats, PaginatedResponse } from '../types'

export default function Agents() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [health, setHealth] = useState<AgentHealthStats | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      api.get<PaginatedResponse<Agent>>('/agents', { params: { limit: 50 } }),
      api.get<{ data: AgentHealthStats }>('/dashboard/agents'),
    ]).then(([agentsRes, healthRes]) => {
      setAgents(agentsRes.data.data)
      setHealth(healthRes.data.data)
    }).catch(() => {})
    .finally(() => setLoading(false))
  }, [])

  const formatTimeAgo = (dateStr: string | null): string => {
    if (!dateStr) return 'Never'
    try {
      const diff = Date.now() - new Date(dateStr).getTime()
      const sec = Math.floor(diff / 1000)
      if (sec < 60) return `${sec}s ago`
      const min = Math.floor(sec / 60)
      if (min < 60) return `${min}m ago`
      const hrs = Math.floor(min / 60)
      if (hrs < 24) return `${hrs}h ago`
      const days = Math.floor(hrs / 24)
      return `${days}d ago`
    } catch {
      return '—'
    }
  }

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ marginBottom: 32 }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.24px', marginBottom: 4 }}>
          代理监控
        </h1>
        <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>监控代理连接状态</p>
      </div>

      {/* KPI Cards */}
      {health && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 12, marginBottom: 32 }}>
          <AgentKpi label="总数" value={health.total} color="var(--text-secondary)" />
          <AgentKpi label="在线" value={health.online} color="var(--success)" dot />
          <AgentKpi label="离线" value={health.offline} color="var(--text-quaternary)" dot />
          <AgentKpi label="降级" value={health.degraded} color="var(--warning)" dot />
          <AgentKpi label="异常" value={health.error} color="var(--danger)" dot />
        </div>
      )}

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
          <div style={{
            width: 24, height: 24, border: '2px solid var(--border-default)',
            borderTopColor: 'var(--brand)', borderRadius: '50%', animation: 'spin 0.6s linear infinite'
          }} />
        </div>
      ) : (
        /* Agent Table */
        <div style={{
          background: 'var(--bg-surface)', borderRadius: 10, border: '1px solid var(--border-subtle)',
          overflow: 'hidden'
        }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                {['名称', '主机名', '状态', '版本', '最后心跳', '更新时间'].map(h => (
                  <th key={h} style={{
                    padding: '10px 16px', textAlign: 'left', fontSize: 11, fontWeight: 500,
                    color: 'var(--text-quaternary)', textTransform: 'uppercase', letterSpacing: '0.5px'
                  }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {agents.length === 0 ? (
                <tr>
                  <td colSpan={6} style={{ padding: '60px 16px', textAlign: 'center' }}>
                    <div style={{ fontSize: 32, marginBottom: 8 }}>🤖</div>
                    <div style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>暂无代理注册</div>
                    <div style={{ fontSize: 12, color: 'var(--text-quaternary)', marginTop: 4 }}>
                      部署代理以开始监控
                    </div>
                  </td>
                </tr>
              ) : (
                agents.map(a => (
                  <tr key={a.id}
                    style={{
                      borderBottom: '1px solid var(--border-subtle)',
                      transition: 'background .1s'
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = 'rgba(255,255,255,0.02)'}
                    onMouseLeave={e => e.currentTarget.style.background = 'transparent'}
                  >
                    <td style={{ padding: '10px 16px', fontSize: 13, fontWeight: 500, color: 'var(--text-primary)' }}>
                      {a.name}
                    </td>
                    <td style={{ padding: '10px 16px' }}>
                      <span style={{
                        fontFamily: 'JetBrains Mono, monospace', fontSize: 12,
                        background: 'rgba(255,255,255,0.05)', color: 'var(--text-secondary)',
                        padding: '2px 8px', borderRadius: 4
                      }}>{a.hostname}</span>
                    </td>
                    <td style={{ padding: '10px 16px' }}>
                      <AgentStatusBadge status={a.status} />
                    </td>
                    <td style={{ padding: '10px 16px' }}>
                      <span style={{
                        fontFamily: 'JetBrains Mono, monospace', fontSize: 11,
                        color: 'var(--text-quaternary)', background: 'rgba(255,255,255,0.04)',
                        padding: '1px 6px', borderRadius: 3
                      }}>{a.version || '—'}</span>
                    </td>
                    <td style={{ padding: '10px 16px', fontSize: 12, color: 'var(--text-tertiary)' }}>
                      {formatTimeAgo(a.last_heartbeat)}
                    </td>
                    <td style={{ padding: '10px 16px', fontSize: 12, color: 'var(--text-quaternary)' }}>
                      {fmtDate(a.updated_at)}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

function AgentKpi({ label, value, color, dot }: { label: string; value: number; color: string; dot?: boolean }) {
  return (
    <div style={{
      background: 'var(--bg-surface)', borderRadius: 10, padding: '16px 20px',
      border: '1px solid var(--border-subtle)', borderLeft: `3px solid ${color}`
    }}>
      <div style={{ fontSize: 12, color: 'var(--text-tertiary)', fontWeight: 500, marginBottom: 6, display: 'flex', alignItems: 'center', gap: 6 }}>
        {dot && <span style={{ width: 6, height: 6, borderRadius: '50%', background: color }} />}
        {label}
      </div>
      <div style={{ fontSize: 24, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.5px' }}>{value}</div>
    </div>
  )
}

function AgentStatusBadge({ status }: { status: string }) {
  const c: Record<string, { bg: string; color: string; label: string }> = {
    online: { bg: 'rgba(39,166,68,0.1)', color: '#4ade80', label: '在线' },
    offline: { bg: 'rgba(255,255,255,0.05)', color: '#8a8f98', label: '离线' },
    error: { bg: 'rgba(239,68,68,0.1)', color: '#f87171', label: '异常' },
    degraded: { bg: 'rgba(245,158,11,0.1)', color: '#fbbf24', label: '降级' },
  }
  const s = c[status] || { bg: 'rgba(255,255,255,0.05)', color: 'var(--text-tertiary)', label: status }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.color }} />
      <span style={{ color: s.color }}>{s.label}</span>
    </span>
  )
}

function fmtDate(d: string) {
  try { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) } catch { return d }
}
