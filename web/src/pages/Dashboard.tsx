import { useEffect, useState } from 'react'
import api from '../api/client'

export default function Dashboard() {
  const [stats, setStats] = useState<Record<string,unknown>|null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.get('/dashboard/overview').then(({data}) => { setStats(data.data); setLoading(false) })
      .catch(() => setLoading(false))
  }, [])

  const s = stats as Record<string,unknown>|null

  return (
    <div style={{ padding:32, maxWidth:1200 }}>
      <div style={{ marginBottom:32 }}>
        <h1 style={{ fontSize:20, fontWeight:600, color:'var(--text-primary)', letterSpacing:'-0.24px', marginBottom:4 }}>
          Dashboard
        </h1>
        <p style={{ fontSize:13, color:'var(--text-tertiary)' }}>IT asset overview</p>
      </div>

      {loading ? (
        <div style={{ display:'flex', justifyContent:'center', padding:80 }}>
          <div style={{ width:24, height:24, border:'2px solid var(--border-default)', borderTopColor:'var(--brand)', borderRadius:'50%', animation:'spin 0.6s linear infinite' }} />
        </div>
      ) : (
        <>
          {/* KPI Cards */}
          <div style={{ display:'grid', gridTemplateColumns:'repeat(4,1fr)', gap:16, marginBottom:32 }}>
            <KpiCard label="Total Assets" value={String(s?.total_assets||0)} color="var(--brand)" />
            <KpiCard label="Online Agents" value="0" color="var(--success)" />
            <KpiCard label="Pending" value="0" color="var(--warning)" />
            <KpiCard label="Available %" value="—" color="#7170ff" />
          </div>

          {/* Charts */}
          <div style={{ display:'grid', gridTemplateColumns:'1fr 1fr', gap:16 }}>
            <Panel title="Status Distribution">
              {s?.by_status ? Object.entries(s.by_status as Record<string,number>).map(([k,v]) => {
                const total = Object.values(s.by_status as Record<string,number>).reduce((a,b)=>a+b,0)||1
                const pct = Math.round(v/total*100)
                return (
                  <div key={k} style={{ marginBottom:12 }}>
                    <div style={{ display:'flex', justifyContent:'space-between', marginBottom:4, fontSize:13 }}>
                      <span style={{ color:'var(--text-secondary)' }}>{statusLabel(k)}</span>
                      <span style={{ fontWeight:500, color:'var(--text-primary)' }}>{v}</span>
                    </div>
                    <div style={{ height:4, background:'rgba(255,255,255,0.05)', borderRadius:2, overflow:'hidden' }}>
                      <div style={{ height:'100%', width:`${pct}%`, background:statusColor(k), borderRadius:2, transition:'width .3s' }} />
                    </div>
                  </div>
                )
              }) : <EmptyState />}
            </Panel>

            <Panel title="Lifecycle Distribution">
              {s?.by_lifecycle ? (
                <div style={{ display:'grid', gridTemplateColumns:'1fr 1fr', gap:8 }}>
                  {Object.entries(s.by_lifecycle as Record<string,number>).map(([k,v]) => (
                    <div key={k} style={{ background:'rgba(255,255,255,0.02)', borderRadius:8, padding:14, border:'1px solid var(--border-subtle)' }}>
                      <div style={{ fontSize:24, fontWeight:600, color:'var(--text-primary)' }}>{v}</div>
                      <div style={{ fontSize:12, color:'var(--text-tertiary)', marginTop:2 }}>{lifecycleLabel(k)}</div>
                    </div>
                  ))}
                </div>
              ) : <EmptyState />}
            </Panel>
          </div>
        </>
      )}
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

function KpiCard({ label, value, color }: { label:string; value:string; color:string }) {
  return (
    <div style={{
      background:'var(--bg-surface)', borderRadius:10, padding:'20px 24px',
      border:'1px solid var(--border-subtle)', borderLeft:`3px solid ${color}`
    }}>
      <div style={{ fontSize:12, color:'var(--text-tertiary)', fontWeight:500, marginBottom:8 }}>{label}</div>
      <div style={{ fontSize:28, fontWeight:600, color:'var(--text-primary)', letterSpacing:'-0.5px' }}>{value}</div>
    </div>
  )
}

function Panel({ title, children }: { title:string; children:React.ReactNode }) {
  return (
    <div style={{
      background:'var(--bg-surface)', borderRadius:10, padding:24,
      border:'1px solid var(--border-subtle)'
    }}>
      <h3 style={{ fontSize:14, fontWeight:600, color:'var(--text-secondary)', marginBottom:16, letterSpacing:'-0.15px' }}>{title}</h3>
      {children}
    </div>
  )
}

function EmptyState() {
  return <div style={{ textAlign:'center', padding:40, color:'var(--text-quaternary)', fontSize:13 }}>No data available</div>
}

function statusLabel(s:string) {
  const m: Record<string,string> = { available:'Available', assigned:'Assigned', maintenance:'Maintenance' }
  return m[s]||s
}
function statusColor(s:string) {
  const m: Record<string,string> = { available:'var(--success)', assigned:'var(--brand)', maintenance:'var(--warning)' }
  return m[s]||'var(--text-quaternary)'
}
function lifecycleLabel(s:string) {
  const m: Record<string,string> = { procurement:'Procurement', deployment:'Deployment', utilization:'In Use', maintenance:'Maintenance', retirement:'Retired' }
  return m[s]||s
}
