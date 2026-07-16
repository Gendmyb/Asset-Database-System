// Assets — Linear-style data table with search, filter, pagination
import { useEffect, useState, useCallback } from 'react'
import api from '../api/client'
import type { Asset, PaginatedResponse } from '../types'

export default function Assets() {
  const [assets, setAssets] = useState<Asset[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [hasMore, setHasMore] = useState(false)
  const [cursor, setCursor] = useState<string|null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [selected, setSelected] = useState<Asset|null>(null)

  const fetchAssets = useCallback(async (reset:boolean) => {
    setLoading(true)
    try {
      const { data } = await api.get<PaginatedResponse<Asset>>('/assets', {
        params: { search:search||undefined, status:statusFilter||undefined, cursor:reset?undefined:cursor, limit:20 }
      })
      setAssets(p => reset?data.data:[...p,...data.data])
      setCursor(data.pagination.next_cursor)
      setHasMore(data.pagination.has_more)
    } catch {}
    setLoading(false)
  }, [search, statusFilter, cursor])

  useEffect(() => { fetchAssets(true) }, [search, statusFilter])

  return (
    <div style={{ padding:32, maxWidth:1400 }}>
      {/* Header */}
      <div style={{ display:'flex', alignItems:'center', justifyContent:'space-between', marginBottom:24 }}>
        <div>
          <h1 style={{ fontSize:20, fontWeight:600, color:'var(--text-primary)', letterSpacing:'-0.24px', marginBottom:4 }}>Assets</h1>
          <p style={{ fontSize:13, color:'var(--text-tertiary)' }}>Manage and track all IT assets</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          style={{
            padding:'8px 16px', borderRadius:6, border:'none', cursor:'pointer',
            background:'var(--brand)', color:'white', fontSize:13, fontWeight:500,
            fontFamily:'inherit', transition:'background .15s'
          }}
          onMouseEnter={e => e.currentTarget.style.background = 'var(--brand-hover)'}
          onMouseLeave={e => e.currentTarget.style.background = 'var(--brand)'}
        >+ New Asset</button>
      </div>

      {/* Search + Filter */}
      <div style={{ display:'flex', gap:12, marginBottom:16 }}>
        <div style={{ position:'relative', flex:1, maxWidth:360 }}>
          <svg style={{ position:'absolute', left:10, top:'50%', transform:'translateY(-50%)', width:14, height:14, color:'var(--text-quaternary)' }}
            viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
          </svg>
          <input
            type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder="Search assets..."
            style={{
              width:'100%', padding:'7px 12px 7px 32px', borderRadius:6,
              border:'1px solid var(--border-default)', background:'rgba(255,255,255,0.02)',
              color:'var(--text-primary)', fontSize:13, outline:'none', fontFamily:'inherit',
            }}
          />
        </div>
        <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)}
          style={{
            padding:'7px 12px', borderRadius:6, border:'1px solid var(--border-default)',
            background:'rgba(255,255,255,0.02)', color:'var(--text-secondary)', fontSize:13,
            outline:'none', fontFamily:'inherit', cursor:'pointer'
          }}>
          <option value="">All statuses</option>
          <option value="available">Available</option>
          <option value="assigned">Assigned</option>
          <option value="maintenance">Maintenance</option>
        </select>
      </div>

      {/* Table */}
      <div style={{
        background:'var(--bg-surface)', borderRadius:10, border:'1px solid var(--border-subtle)',
        overflow:'hidden'
      }}>
        <table style={{ width:'100%', borderCollapse:'collapse' }}>
          <thead>
            <tr style={{ borderBottom:'1px solid var(--border-subtle)' }}>
              {['Tag','Name','Manufacturer','Model','Status','Lifecycle','Updated'].map(h => (
                <th key={h} style={{ padding:'10px 16px', textAlign:'left', fontSize:11, fontWeight:500,
                  color:'var(--text-quaternary)', textTransform:'uppercase', letterSpacing:'0.5px' }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {assets.length === 0 && !loading && (
              <tr>
                <td colSpan={7} style={{ padding:'60px 16px', textAlign:'center' }}>
                  <div style={{ fontSize:32, marginBottom:8 }}>📦</div>
                  <div style={{ fontSize:13, color:'var(--text-tertiary)' }}>No assets yet</div>
                  <div style={{ fontSize:12, color:'var(--text-quaternary)', marginTop:4 }}>
                    Create your first asset to get started
                  </div>
                </td>
              </tr>
            )}
            {assets.map(a => (
              <tr key={a.id} onClick={() => setSelected(a)}
                style={{ borderBottom:'1px solid var(--border-subtle)', cursor:'pointer',
                  transition:'background .1s' }}
                onMouseEnter={e => e.currentTarget.style.background = 'rgba(255,255,255,0.02)'}
                onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                <td style={{ padding:'10px 16px' }}>
                  <span style={{ fontFamily:'JetBrains Mono, monospace', fontSize:12,
                    background:'rgba(255,255,255,0.05)', color:'var(--text-secondary)',
                    padding:'2px 8px', borderRadius:4 }}>{a.asset_tag}</span>
                </td>
                <td style={{ padding:'10px 16px', fontSize:13, fontWeight:500, color:'var(--text-primary)' }}>{a.name}</td>
                <td style={{ padding:'10px 16px', fontSize:13, color:'var(--text-tertiary)' }}>{a.manufacturer||'—'}</td>
                <td style={{ padding:'10px 16px', fontSize:13, color:'var(--text-tertiary)' }}>{a.model||'—'}</td>
                <td style={{ padding:'10px 16px' }}><StatusBadge status={a.status} /></td>
                <td style={{ padding:'10px 16px', fontSize:13, color:'var(--text-tertiary)' }}>{lifecycleLabel(a.lifecycle_state)}</td>
                <td style={{ padding:'10px 16px', fontSize:12, color:'var(--text-quaternary)' }}>{fmtDate(a.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>

        {loading && (
          <div style={{ display:'flex', justifyContent:'center', padding:32 }}>
            <div style={{ width:20, height:20, border:'2px solid var(--border-default)', borderTopColor:'var(--brand)', borderRadius:'50%', animation:'spin .6s linear infinite' }} />
          </div>
        )}
        {hasMore && !loading && (
          <button onClick={() => fetchAssets(false)}
            style={{
              width:'100%', padding:'10px', border:'none', background:'none', cursor:'pointer',
              color:'var(--text-tertiary)', fontSize:13, fontFamily:'inherit',
              borderTop:'1px solid var(--border-subtle)'
            }}
            onMouseEnter={e => e.currentTarget.style.color = 'var(--text-secondary)'}
            onMouseLeave={e => e.currentTarget.style.color = 'var(--text-tertiary)'}
          >Load more...</button>
        )}
      </div>

      {/* Create Modal */}
      {showCreate && <CreateModal
        loading={createLoading}
        onClose={() => setShowCreate(false)}
        onSubmit={async (form) => {
          setCreateLoading(true)
          try { await api.post('/assets', form); setShowCreate(false); fetchAssets(true) }
          catch {}
          setCreateLoading(false)
        }}
      />}

      {/* Detail Panel */}
      {selected && <DetailPanel asset={selected} onClose={() => setSelected(null)} />}

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

// --- Sub-components ---

function StatusBadge({ status }: { status: string }) {
  const c: Record<string,{bg:string;color:string;label:string}> = {
    available: {bg:'rgba(39,166,68,0.1)', color:'#4ade80', label:'Available'},
    assigned: {bg:'rgba(94,106,210,0.1)', color:'#818cf8', label:'Assigned'},
    maintenance: {bg:'rgba(245,158,11,0.1)', color:'#fbbf24', label:'Maintenance'},
  }
  const s = c[status]||{bg:'rgba(255,255,255,0.05)', color:'var(--text-tertiary)', label:status}
  return (
    <span style={{ display:'inline-flex', alignItems:'center', gap:6, fontSize:12 }}>
      <span style={{ width:6, height:6, borderRadius:'50%', background:s.color }} />
      <span style={{ color:s.color }}>{s.label}</span>
    </span>
  )
}

function lifecycleLabel(s:string) {
  const m: Record<string,string> = { procurement:'Procurement', deployment:'Deployment', utilization:'In Use', maintenance:'Maintenance', retirement:'Retired' }
  return m[s]||s
}

function fmtDate(d:string) {
  try { return new Date(d).toLocaleDateString('en-US',{month:'short',day:'numeric',year:'numeric'}) }
  catch { return d }
}

function CreateModal({ loading, onClose, onSubmit }: {
  loading:boolean; onClose:()=>void; onSubmit:(f:Record<string,string>)=>void
}) {
  const [form, setForm] = useState({ asset_tag:'', name:'', manufacturer:'', model:'', serial_number:'', type_id:'type-001' })

  return (
    <div style={{
      position:'fixed', inset:0, zIndex:50, display:'flex', alignItems:'center', justifyContent:'center',
      background:'rgba(0,0,0,0.6)', backdropFilter:'blur(4px)'
    }} onClick={onClose}>
      <div style={{
        background:'var(--bg-surface)', borderRadius:12, border:'1px solid var(--border-default)',
        width:'100%', maxWidth:440, margin:'0 16px'
      }} onClick={e => e.stopPropagation()}>
        <div style={{ display:'flex', alignItems:'center', justifyContent:'space-between', padding:'16px 24px', borderBottom:'1px solid var(--border-subtle)' }}>
          <h3 style={{ fontSize:15, fontWeight:600, color:'var(--text-primary)' }}>New Asset</h3>
          <button onClick={onClose} style={{ background:'none', border:'none', color:'var(--text-quaternary)', cursor:'pointer', fontSize:16 }}>✕</button>
        </div>
        <form onSubmit={e => { e.preventDefault(); onSubmit(form) }} style={{ padding:24 }}>
          {[
            {k:'asset_tag',l:'Asset Tag',r:true},
            {k:'name',l:'Name',r:true},
            {k:'manufacturer',l:'Manufacturer'},
            {k:'model',l:'Model'},
            {k:'serial_number',l:'Serial Number'},
          ].map(f => (
            <div key={f.k} style={{ marginBottom:14 }}>
              <label style={{ display:'block', fontSize:12, fontWeight:500, color:'var(--text-secondary)', marginBottom:5 }}>
                {f.l}{f.r && <span style={{ color:'var(--danger)', marginLeft:2 }}>*</span>}
              </label>
              <input
                value={(form as any)[f.k]} onChange={e => setForm({...form,[f.k]:e.target.value})}
                style={{
                  width:'100%', padding:'7px 10px', borderRadius:5, border:'1px solid var(--border-default)',
                  background:'rgba(255,255,255,0.02)', color:'var(--text-primary)', fontSize:13,
                  outline:'none', fontFamily:'inherit'
                }}
                required={f.r}
              />
            </div>
          ))}
          <div style={{ display:'flex', gap:10, marginTop:20 }}>
            <button type="button" onClick={onClose}
              style={{
                flex:1, padding:'8px', borderRadius:6, border:'1px solid var(--border-default)',
                background:'transparent', color:'var(--text-secondary)', fontSize:13, fontWeight:500,
                cursor:'pointer', fontFamily:'inherit'
              }}>Cancel</button>
            <button type="submit" disabled={loading}
              style={{
                flex:1, padding:'8px', borderRadius:6, border:'none', cursor:loading?'default':'pointer',
                background:loading?'rgba(94,106,210,0.4)':'var(--brand)',
                color:'white', fontSize:13, fontWeight:500, fontFamily:'inherit'
              }}>{loading?'Creating...':'Create'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

function DetailPanel({ asset, onClose }: { asset:Asset; onClose:()=>void }) {
  return (
    <div style={{
      position:'fixed', top:0, right:0, bottom:0, width:360, zIndex:40,
      background:'var(--bg-surface)', borderLeft:'1px solid var(--border-default)',
      boxShadow:'-8px 0 24px rgba(0,0,0,0.4)', overflow:'auto'
    }}>
      <div style={{ position:'sticky', top:0, background:'var(--bg-surface)', borderBottom:'1px solid var(--border-subtle)',
        padding:'16px 20px', display:'flex', alignItems:'center', justifyContent:'space-between' }}>
        <h3 style={{ fontSize:14, fontWeight:600, color:'var(--text-primary)' }}>Asset Details</h3>
        <button onClick={onClose} style={{ background:'none', border:'none', color:'var(--text-quaternary)', cursor:'pointer', fontSize:16 }}>✕</button>
      </div>
      <div style={{ padding:20 }}>
        <div style={{ textAlign:'center', marginBottom:24 }}>
          <div style={{
            width:56, height:56, background:'rgba(94,106,210,0.1)', borderRadius:14,
            display:'inline-flex', alignItems:'center', justifyContent:'center', marginBottom:12
          }}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--brand)" strokeWidth="1.5">
              <path d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
            </svg>
          </div>
          <div style={{ fontSize:16, fontWeight:600, color:'var(--text-primary)' }}>{asset.name}</div>
          <div style={{ fontFamily:'JetBrains Mono, monospace', fontSize:12, color:'var(--text-tertiary)', marginTop:4 }}>{asset.asset_tag}</div>
        </div>
        {[
          ['Manufacturer',asset.manufacturer],['Model',asset.model],['Serial Number',asset.serial_number],
          ['Status',asset.status],['Lifecycle',lifecycleLabel(asset.lifecycle_state)],
          ['Version',`v${asset.version}`],['Created',fmtDate(asset.created_at)],
        ].map(([k,v]) => (
          <div key={k as string} style={{ display:'flex', justifyContent:'space-between', padding:'10px 0', borderBottom:'1px solid var(--border-subtle)' }}>
            <span style={{ fontSize:12, color:'var(--text-tertiary)' }}>{k}</span>
            <span style={{ fontSize:12, fontWeight:500, color:'var(--text-primary)' }}>{v||'—'}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
