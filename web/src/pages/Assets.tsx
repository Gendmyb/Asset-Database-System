// Assets — Linear-style data table with search, filter, pagination, edit, lifecycle
import { useEffect, useState, useCallback } from 'react'
import api from '../api/client'
import type { Asset, PaginatedResponse } from '../types'

const LIFECYCLE_ORDER = ['procurement', 'deployment', 'utilization', 'maintenance', 'retirement'] as const
const LIFECYCLE_LABELS: Record<string, string> = {
  procurement: '采购中', deployment: '部署中',
  utilization: '使用中', maintenance: '维护中', retirement: '已退役'
}

// Which transitions are allowed from each state
const ALLOWED_TRANSITIONS: Record<string, string[]> = {
  procurement: ['deployment'],
  deployment: ['utilization', 'maintenance'],
  utilization: ['maintenance', 'retirement'],
  maintenance: ['utilization', 'retirement'],
  retirement: [],
}

export default function Assets() {
  const [assets, setAssets] = useState<Asset[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [lifecycleFilter, setLifecycleFilter] = useState('')
  const [manufacturerFilter, setManufacturerFilter] = useState('')
  const [hasMore, setHasMore] = useState(false)
  const [cursor, setCursor] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createLoading, setCreateLoading] = useState(false)
  const [selected, setSelected] = useState<Asset | null>(null)

  const fetchAssets = useCallback(async (reset: boolean) => {
    setLoading(true)
    try {
      const { data } = await api.get<PaginatedResponse<Asset>>('/assets', {
        params: {
          search: search || undefined,
          status: statusFilter || undefined,
          lifecycle: lifecycleFilter || undefined,
          manufacturer: manufacturerFilter || undefined,
          cursor: reset ? undefined : cursor,
          limit: 20
        }
      })
      setAssets(p => reset ? data.data : [...p, ...data.data])
      setCursor(data.pagination.next_cursor)
      setHasMore(data.pagination.has_more)
    } catch { }
    setLoading(false)
  }, [search, statusFilter, lifecycleFilter, manufacturerFilter, cursor])

  useEffect(() => { fetchAssets(true) }, [search, statusFilter, lifecycleFilter, manufacturerFilter])

  // Refresh selected asset in place after edit/lifecycle/assign
  const refreshAsset = useCallback(async (id: string) => {
    try {
      const { data } = await api.get<Asset>(`/assets/${id}`)
      const asset = (data as any).data || data
      setSelected(asset)
      setAssets(prev => prev.map(a => a.id === id ? asset : a))
      // Also fetch assigned user if status is 'assigned'
      if (asset.status === 'assigned') {
        api.get(`/assets/${id}/assignments`).then(({ data: ad }: any) => {
          const assignment = ad?.data || ad
          const userId = assignment?.assigned_to
          if (userId) {
            api.get(`/users/${userId}`).then(({ data: ud }: any) => {
              const name = ud?.data?.username || ud?.username || userId
              setSelected((prev: any) => prev ? { ...prev, _assignedUser: name } : prev)
            }).catch(() => {})
          }
        }).catch(() => {})
      }
    } catch { }
  }, [])

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.24px', marginBottom: 4 }}>Assets</h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>Manage and track all IT assets</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          style={{
            padding: '8px 16px', borderRadius: 6, border: 'none', cursor: 'pointer',
            background: 'var(--brand)', color: 'white', fontSize: 13, fontWeight: 500,
            fontFamily: 'inherit', transition: 'background .15s'
          }}
          onMouseEnter={e => e.currentTarget.style.background = 'var(--brand-hover)'}
          onMouseLeave={e => e.currentTarget.style.background = 'var(--brand)'}
        >+ 新建资产</button>
      </div>

      {/* Search + Filter */}
      <div style={{ display: 'flex', gap: 10, marginBottom: 16, flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: 220, maxWidth: 360 }}>
          <svg style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', width: 14, height: 14, color: 'var(--text-quaternary)' }}
            viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="11" cy="11" r="8" /><path d="m21 21-4.35-4.35" />
          </svg>
          <input
            type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder="搜索名称、标签、制造商..."
            style={{
              width: '100%', padding: '7px 12px 7px 32px', borderRadius: 6,
              border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.02)',
              color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit',
            }}
          />
        </div>
        <FilterSelect label="状态" value={statusFilter} onChange={setStatusFilter} options={[
          ['','全部'],['available','可用'],['assigned','已领用'],['maintenance','维护中']
        ]} />
        <FilterSelect label="生命周期" value={lifecycleFilter} onChange={setLifecycleFilter} options={[
          ['','全部'],['procurement','采购中'],['deployment','部署中'],['utilization','使用中'],['maintenance','维护中'],['retirement','已退役']
        ]} />
        <FilterSelect label="制造商" value={manufacturerFilter} onChange={setManufacturerFilter} options={[
          ['','全部'],['Apple','Apple'],['Dell','Dell'],['Lenovo','Lenovo'],['Cisco','Cisco']
        ]} />
      </div>

      {/* Table */}
      <div style={{
        background: 'var(--bg-surface)', borderRadius: 10, border: '1px solid var(--border-subtle)',
        overflow: 'hidden'
      }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border-subtle)' }}>
              {['编号', '名称', '制造商', '型号', '状态', '生命周期', '更新时间'].map(h => (
                <th key={h} style={{
                  padding: '10px 16px', textAlign: 'left', fontSize: 11, fontWeight: 500,
                  color: 'var(--text-quaternary)', textTransform: 'uppercase', letterSpacing: '0.5px'
                }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {assets.length === 0 && !loading && (
              <tr>
                <td colSpan={7} style={{ padding: '60px 16px', textAlign: 'center' }}>
                  <div style={{ fontSize: 32, marginBottom: 8 }}>📦</div>
                  <div style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>暂无资产</div>
                  <div style={{ fontSize: 12, color: 'var(--text-quaternary)', marginTop: 4 }}>
                    Create your first asset to get started
                  </div>
                </td>
              </tr>
            )}
            {assets.map(a => (
              <tr key={a.id} onClick={() => setSelected(a)}
                style={{
                  borderBottom: '1px solid var(--border-subtle)', cursor: 'pointer',
                  transition: 'background .1s'
                }}
                onMouseEnter={e => e.currentTarget.style.background = 'rgba(255,255,255,0.02)'}
                onMouseLeave={e => e.currentTarget.style.background = 'transparent'}>
                <td style={{ padding: '10px 16px' }}>
                  <span style={{
                    fontFamily: 'JetBrains Mono, monospace', fontSize: 12,
                    background: 'rgba(255,255,255,0.05)', color: 'var(--text-secondary)',
                    padding: '2px 8px', borderRadius: 4
                  }}>{a.asset_tag}</span>
                </td>
                <td style={{ padding: '10px 16px', fontSize: 13, fontWeight: 500, color: 'var(--text-primary)' }}>{a.name}</td>
                <td style={{ padding: '10px 16px', fontSize: 13, color: 'var(--text-tertiary)' }}>{a.manufacturer || '—'}</td>
                <td style={{ padding: '10px 16px', fontSize: 13, color: 'var(--text-tertiary)' }}>{a.model || '—'}</td>
                <td style={{ padding: '10px 16px' }}><StatusBadge status={a.status} /></td>
                <td style={{ padding: '10px 16px', fontSize: 13, color: 'var(--text-tertiary)' }}>{lifecycleLabel(a.lifecycle_state)}</td>
                <td style={{ padding: '10px 16px', fontSize: 12, color: 'var(--text-quaternary)' }}>{fmtDate(a.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>

        {loading && (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
            <div style={{ width: 20, height: 20, border: '2px solid var(--border-default)', borderTopColor: 'var(--brand)', borderRadius: '50%', animation: 'spin .6s linear infinite' }} />
          </div>
        )}
        {hasMore && !loading && (
          <button onClick={() => fetchAssets(false)}
            style={{
              width: '100%', padding: '10px', border: 'none', background: 'none', cursor: 'pointer',
              color: 'var(--text-tertiary)', fontSize: 13, fontFamily: 'inherit',
              borderTop: '1px solid var(--border-subtle)'
            }}
            onMouseEnter={e => e.currentTarget.style.color = 'var(--text-secondary)'}
            onMouseLeave={e => e.currentTarget.style.color = 'var(--text-tertiary)'}
          >加载更多...</button>
        )}
      </div>

      {/* Create Modal */}
      {showCreate && <CreateModal
        loading={createLoading}
        onClose={() => setShowCreate(false)}
        onSubmit={async (form) => {
          setCreateLoading(true)
          try { await api.post('/assets', form); setShowCreate(false); fetchAssets(true) }
          catch { }
          setCreateLoading(false)
        }}
      />}

      {/* Detail Panel with Overlay */}
      {selected && (
        <>
          {/* Backdrop — 点击关闭 */}
          <div
            onClick={() => setSelected(null)}
            style={{
              position: 'fixed', inset: 0, zIndex: 49,
              background: 'rgba(0,0,0,0.5)', backdropFilter: 'blur(4px)',
            }}
          />
          <DetailPanel
            asset={selected}
            onClose={() => setSelected(null)}
            onRefresh={refreshAsset}
          />
        </>
      )}

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

// --- Sub-components ---

function StatusBadge({ status }: { status: string }) {
  const c: Record<string, { bg: string; color: string; label: string }> = {
    available: { bg: 'rgba(39,166,68,0.1)', color: '#4ade80', label: '可用' },
    assigned: { bg: 'rgba(94,106,210,0.1)', color: '#818cf8', label: '已领用' },
    maintenance: { bg: 'rgba(245,158,11,0.1)', color: '#fbbf24', label: '维护中' },
  }
  const s = c[status] || { bg: 'rgba(255,255,255,0.05)', color: 'var(--text-tertiary)', label: status }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
      <span style={{ width: 6, height: 6, borderRadius: '50%', background: s.color }} />
      <span style={{ color: s.color }}>{s.label}</span>
    </span>
  )
}

function lifecycleLabel(s: string) {
  return LIFECYCLE_LABELS[s] || s
}

function fmtDate(d: string) {
  try { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) }
  catch { return d }
}

function CreateModal({ loading, onClose, onSubmit }: {
  loading: boolean; onClose: () => void; onSubmit: (f: Record<string, string>) => void
}) {
  const [form, setForm] = useState({
    asset_tag: '', name: '', manufacturer: '', model: '',
    serial_number: '', type_id: '10000000-0000-4000-a000-000000000001', managed_by: ''
  })

  return (
    <div style={{
      position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(4px)'
    }} onClick={onClose}>
      <div style={{
        background: '#111213', borderRadius: 12, border: '1px solid var(--border-default)',
        width: '100%', maxWidth: 440, margin: '0 16px'
      }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 24px', borderBottom: '1px solid var(--border-subtle)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>新建资产</h3>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: 'var(--text-quaternary)', cursor: 'pointer', fontSize: 16 }}>✕</button>
        </div>
        <form onSubmit={e => { e.preventDefault(); onSubmit(form) }} style={{ padding: 24 }}>
          {[
            { k: 'asset_tag', l: '资产编号', hint: '留空自动生成' },
            { k: 'name', l: '名称', r: true },
            { k: 'manufacturer', l: '制造商' },
            { k: 'model', l: '型号' },
            { k: 'serial_number', l: '序列号' },
            { k: 'managed_by', l: '管理人' },
          ].map(f => (
            <div key={f.k} style={{ marginBottom: 14 }}>
              <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
                {f.l}{f.r && <span style={{ color: 'var(--danger)', marginLeft: 2 }}>*</span>}
                {(f as any).hint && <span style={{ color: 'var(--text-quaternary)', marginLeft: 6, fontSize: 11 }}>{(f as any).hint}</span>}
              </label>
              <input
                value={(form as any)[f.k]} onChange={e => setForm({ ...form, [f.k]: e.target.value })}
                style={{
                  width: '100%', padding: '7px 10px', borderRadius: 5, border: '1px solid var(--border-default)',
                  background: 'rgba(255,255,255,0.02)', color: 'var(--text-primary)', fontSize: 13,
                  outline: 'none', fontFamily: 'inherit'
                }}
                required={f.r}
              />
            </div>
          ))}

          {/* 资产类型下拉 */}
          <div style={{ marginBottom: 14 }}>
            <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
              资产类型
            </label>
            <select
              value={form.type_id}
              onChange={e => setForm({ ...form, type_id: e.target.value })}
              style={{
                width: '100%', padding: '7px 10px', borderRadius: 5,
                border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit', cursor: 'pointer'
              }}>
              <option value="10000000-0000-4000-a000-000000000001">通用资产</option>
              <option value="10000000-0000-4000-a000-000000000002">计算机</option>
              <option value="10000000-0000-4000-a000-000000000003">显示器</option>
              <option value="10000000-0000-4000-a000-000000000004">网络设备</option>
            </select>
          </div>

          <div style={{ display: 'flex', gap: 10, marginTop: 20 }}>
            <button type="button" onClick={onClose}
              style={{
                flex: 1, padding: '8px', borderRadius: 6, border: '1px solid var(--border-default)',
                background: 'transparent', color: 'var(--text-secondary)', fontSize: 13, fontWeight: 500,
                cursor: 'pointer', fontFamily: 'inherit'
              }}>取消</button>
            <button type="submit" disabled={loading}
              style={{
                flex: 1, padding: '8px', borderRadius: 6, border: 'none', cursor: loading ? 'default' : 'pointer',
                background: loading ? 'rgba(94,106,210,0.4)' : 'var(--brand)',
                color: 'white', fontSize: 13, fontWeight: 500, fontFamily: 'inherit'
              }}>{loading ? '创建中...' : '创建'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

// --- DetailPanel with editable fields + lifecycle management ---
function DetailPanel({ asset, onClose, onRefresh }: { asset: Asset; onClose: () => void; onRefresh: (id: string) => void }) {
  const [editMode, setEditMode] = useState(false)
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [lifecycleLoading, setLifecycleLoading] = useState(false)
  const [error, setError] = useState('')
  const [assignedUser, setAssignedUser] = useState<string | null>(null)
  const [showAssign, setShowAssign] = useState(false)
  const [releaseLoading, setReleaseLoading] = useState(false)
  const [form, setForm] = useState({
    name: asset.name,
    manufacturer: asset.manufacturer || '',
    model: asset.model || '',
    serial_number: asset.serial_number || '',
    status: asset.status,
  })

  // Reset form when asset changes (new selection)
  useEffect(() => {
    setForm({
      name: asset.name,
      manufacturer: asset.manufacturer || '',
      model: asset.model || '',
      serial_number: asset.serial_number || '',
      status: asset.status,
    })
    setEditMode(false)
    setError('')
    setAssignedUser(null)
  }, [asset.id])

  // Fetch username for assigned_to user via GET /api/v1/users/:id
  useEffect(() => {
    let cancelled = false
    if (asset.status !== 'assigned') {
      setAssignedUser(null)
      return
    }
    // Get active assignment to discover the assigned_to user ID
    api.get(`/assets/${asset.id}/assignments`)
      .then(({ data }: any) => {
        if (cancelled) return
        // API 返回 {data: {assigned_to: "uuid", ...}} — 单个对象
        const assignment = data?.data || data
        const userId = assignment?.assigned_to
        if (userId) {
          api.get(`/users/${userId}`)
            .then(({ data: userData }: any) => {
              if (!cancelled) {
                const name = userData?.data?.username || userData?.username || userId
                setAssignedUser(name)
              }
            })
            .catch(() => { if (!cancelled) setAssignedUser(userId) })
        } else {
          setAssignedUser(null)
        }
      })
      .catch(() => { if (!cancelled) setAssignedUser(null) })
    return () => { cancelled = true }
  }, [asset.id, asset.status])

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      await api.put(`/assets/${asset.id}`, form, {
        headers: { 'If-Match': `"${asset.version}"` }
      })
      setEditMode(false)
      onRefresh(asset.id)
    } catch (err: any) {
      const msg = err?.response?.data?.message || err?.message || 'Save failed'
      setError(msg)
    }
    setSaving(false)
  }

  const handleLifecycleTransition = async (target: string) => {
    setLifecycleLoading(true)
    setError('')
    try {
      await api.put(`/assets/${asset.id}/lifecycle`, {
        lifecycle_state: target,
        version: asset.version,
      })
      onRefresh(asset.id)
    } catch (err: any) {
      const msg = err?.response?.data?.message || err?.message || 'Lifecycle transition failed'
      setError(msg)
    }
    setLifecycleLoading(false)
  }

  const handleRelease = async () => {
    setReleaseLoading(true)
    setError('')
    try {
      await api.post(`/assets/${asset.id}/release`)
      onRefresh(asset.id)
    } catch (err: any) {
      const msg = err?.response?.data?.message || err?.message || 'Release failed'
      setError(msg)
    }
    setReleaseLoading(false)
  }

  const transitions = ALLOWED_TRANSITIONS[asset.lifecycle_state] || []
  const managedBy = (asset.properties?.managed_by as string) || null

  return (
    <div style={{
      position: 'fixed', top: 0, right: 0, bottom: 0, width: 400, zIndex: 50,
      background: '#111213', borderLeft: '1px solid rgba(255,255,255,0.1)',
      boxShadow: '-12px 0 40px rgba(0,0,0,0.6)', overflow: 'auto',
      display: 'flex', flexDirection: 'column',
    }}>
      {/* Sticky Header */}
      <div style={{
        position: 'sticky', top: 0, background: '#111213', borderBottom: '1px solid var(--border-subtle)',
        padding: '16px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', zIndex: 1, flexShrink: 0
      }}>
        <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)' }}>资产详情</h3>
        <div style={{ display: 'flex', gap: 6 }}>
          {!editMode && (
            <button onClick={() => setEditMode(true)}
              style={{
                background: 'transparent', border: '1px solid var(--border-default)', borderRadius: 5,
                color: 'var(--text-secondary)', cursor: 'pointer', fontSize: 12, padding: '4px 10px',
                fontFamily: 'inherit'
              }}>
              编辑
            </button>
          )}
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: 'var(--text-quaternary)', cursor: 'pointer', fontSize: 16 }}>✕</button>
        </div>
      </div>

      <div style={{ padding: 20, flex: 1 }}>
        {/* Icon + Tag */}
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <div style={{
            width: 56, height: 56, background: 'rgba(94,106,210,0.1)', borderRadius: 14,
            display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12
          }}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--brand)" strokeWidth="1.5">
              <path d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          </div>
          <div style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-primary)' }}>{asset.name}</div>
          <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>
            {asset.asset_tag} <span style={{ color: 'var(--text-quaternary)' }}>· v{asset.version}</span>
          </div>
        </div>

        {/* Error banner */}
        {error && (
          <div style={{
            marginBottom: 16, padding: '10px 14px', borderRadius: 6,
            background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.2)',
            color: '#f87171', fontSize: 12
          }}>
            {error}
          </div>
        )}

        {/* Editable Fields */}
        {editMode ? (
          <div style={{ marginBottom: 20 }}>
            {[
              { k: 'name', l: '名称' },
              { k: 'manufacturer', l: '制造商' },
              { k: 'model', l: '型号' },
              { k: 'serial_number', l: '序列号' },
            ].map(f => (
              <div key={f.k} style={{ marginBottom: 12 }}>
                <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
                  {f.l}
                </label>
                <input
                  value={(form as any)[f.k]}
                  onChange={e => setForm({ ...form, [f.k]: e.target.value })}
                  style={{
                    width: '100%', padding: '7px 10px', borderRadius: 5,
                    border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.03)',
                    color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit'
                  }}
                />
              </div>
            ))}

            {/* Status selector */}
            <div style={{ marginBottom: 12 }}>
              <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
                状态
              </label>
              <select
                value={form.status}
                onChange={e => setForm({ ...form, status: e.target.value })}
                style={{
                  width: '100%', padding: '7px 10px', borderRadius: 5,
                  border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.03)',
                  color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit', cursor: 'pointer'
                }}>
                <option value="available">可用</option>
                <option value="assigned">已领用</option>
                <option value="maintenance">维护中</option>
              </select>
            </div>

            {/* Save / Cancel */}
            <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
              <button onClick={() => {
                setEditMode(false)
                setForm({ name: asset.name, manufacturer: asset.manufacturer || '', model: asset.model || '', serial_number: asset.serial_number || '', status: asset.status })
                setError('')
              }}
                style={{
                  flex: 1, padding: '8px', borderRadius: 6, border: '1px solid var(--border-default)',
                  background: 'transparent', color: 'var(--text-secondary)', fontSize: 13, fontWeight: 500,
                  cursor: 'pointer', fontFamily: 'inherit'
                }}>取消</button>
              <button onClick={handleSave} disabled={saving}
                style={{
                  flex: 1, padding: '8px', borderRadius: 6, border: 'none',
                  background: saving ? 'rgba(94,106,210,0.4)' : 'var(--brand)',
                  color: 'white', fontSize: 13, fontWeight: 500, cursor: saving ? 'default' : 'pointer',
                  fontFamily: 'inherit'
                }}>{saving ? '保存中...' : '保存'}</button>
            </div>
          </div>
        ) : (
          /* Read-only fields */
          <>
            {[
              ['制造商', asset.manufacturer],
              ['型号', asset.model],
              ['序列号', asset.serial_number],
              ['状态', asset.status],
              ['使用人', (asset as any)._assignedUser], 
              ['管理人', managedBy],
            ].map(([k, v]) => (
              <div key={k as string} style={{ display: 'flex', justifyContent: 'space-between', padding: '10px 0', borderBottom: '1px solid var(--border-subtle)' }}>
                <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>{k}</span>
                <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-primary)' }}>
                  {k === '状态' ? (
                    <StatusBadge status={v as string} />
                  ) : k === '使用人' ? (
                    v || '未领用'
                  ) : k === '管理人' ? (
                    v || '未指定'
                  ) : (
                    v || '—'
                  )}
                </span>
              </div>
            ))}
          </>
        )}

        {/* Lifecycle Section */}
        <div style={{ marginTop: 24 }}>
          <h4 style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: 12 }}>
            生命周期
          </h4>

          {/* Current lifecycle state */}
          <div style={{
            display: 'flex', alignItems: 'center', gap: 10, marginBottom: 16,
            padding: '10px 14px', borderRadius: 8,
            background: 'rgba(94,106,210,0.08)', border: '1px solid rgba(94,106,210,0.15)'
          }}>
            <div style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--brand)' }} />
            <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--brand)' }}>
              {LIFECYCLE_LABELS[asset.lifecycle_state] || asset.lifecycle_state}
            </span>
          </div>

          {/* Transition buttons */}
          {transitions.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <div style={{ fontSize: 11, color: 'var(--text-quaternary)', marginBottom: 2 }}>可转换状态：</div>
              {transitions.map(target => (
                <button
                  key={target}
                  onClick={() => handleLifecycleTransition(target)}
                  disabled={lifecycleLoading}
                  style={{
                    width: '100%', padding: '8px 14px', borderRadius: 6, cursor: lifecycleLoading ? 'default' : 'pointer',
                    background: lifecycleLoading ? 'rgba(255,255,255,0.02)' : 'rgba(255,255,255,0.03)',
                    border: '1px solid var(--border-default)',
                    color: 'var(--text-secondary)', fontSize: 13, fontWeight: 500,
                    fontFamily: 'inherit', textAlign: 'left',
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                    transition: 'all .15s'
                  }}
                  onMouseEnter={e => {
                    if (!lifecycleLoading) {
                      e.currentTarget.style.background = 'rgba(94,106,210,0.1)'
                      e.currentTarget.style.borderColor = 'rgba(94,106,210,0.3)'
                      e.currentTarget.style.color = 'var(--text-primary)'
                    }
                  }}
                  onMouseLeave={e => {
                    e.currentTarget.style.background = 'rgba(255,255,255,0.03)'
                    e.currentTarget.style.borderColor = 'var(--border-default)'
                    e.currentTarget.style.color = 'var(--text-secondary)'
                  }}
                >
                  <span>→ {LIFECYCLE_LABELS[target] || target}</span>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M5 12h14M12 5l7 7-7 7" />
                  </svg>
                </button>
              ))}
            </div>
          )}

          {transitions.length === 0 && (
            <div style={{ fontSize: 12, color: 'var(--text-quaternary)', padding: '8px 0' }}>
              终态，不可转换
            </div>
          )}
        </div>

        {/* Actions: Assign / Release */}
        <div style={{ marginTop: 24 }}>
          {asset.status === 'available' && (
            <button onClick={() => setShowAssign(true)}
              style={{
                width: '100%', padding: '10px', borderRadius: 6, border: 'none',
                background: 'var(--brand)', color: 'white', fontSize: 13, fontWeight: 500,
                cursor: 'pointer', fontFamily: 'inherit'
              }}
              onMouseEnter={e => e.currentTarget.style.background = 'var(--brand-hover)'}
              onMouseLeave={e => e.currentTarget.style.background = 'var(--brand)'}
            >领用</button>
          )}
          {asset.status === 'assigned' && (
            <button onClick={handleRelease} disabled={releaseLoading}
              style={{
                width: '100%', padding: '10px', borderRadius: 6, border: 'none',
                background: releaseLoading ? 'rgba(94,106,210,0.4)' : 'var(--brand)',
                color: 'white', fontSize: 13, fontWeight: 500,
                cursor: releaseLoading ? 'default' : 'pointer', fontFamily: 'inherit'
              }}
              onMouseEnter={e => { if (!releaseLoading) e.currentTarget.style.background = 'var(--brand-hover)' }}
              onMouseLeave={e => { if (!releaseLoading) e.currentTarget.style.background = 'var(--brand)' }}
            >{releaseLoading ? '归还中...' : '归还'}</button>
          )}
        </div>

        {/* Meta info */}
        <div style={{ marginTop: 24, paddingTop: 16, borderTop: '1px solid var(--border-subtle)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
            <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>创建时间</span>
            <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{fmtDate(asset.created_at)}</span>
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0' }}>
            <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>最后更新</span>
            <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{fmtDate(asset.updated_at)}</span>
          </div>
        </div>
      </div>

      {/* Assign Dialog overlay */}
      {showAssign && (
        <AssignDialog
          asset={asset}
          onClose={() => setShowAssign(false)}
          onRefresh={onRefresh}
        />
      )}
    </div>
  )
}

// AssignDialog — 领用资产弹窗
function AssignDialog({ asset, onClose, onRefresh }: { asset: Asset; onClose: () => void; onRefresh: (id: string) => void }) {
  const [users, setUsers] = useState<{ id: string; username: string }[]>([])
  const [selectedUser, setSelectedUser] = useState('')
  const [notes, setNotes] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api.get('/users')
      .then(({ data }: any) => {
        const list = data?.data || data || []
        setUsers(Array.isArray(list) ? list : [])
      })
      .catch(() => setUsers([]))
  }, [])

  const handleSubmit = async () => {
    if (!selectedUser) return
    setLoading(true)
    setError('')
    try {
      await api.post(`/assets/${asset.id}/assign`, { assigned_to: selectedUser, notes })
      onRefresh(asset.id)
      onClose()
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || 'Assign failed')
    }
    setLoading(false)
  }

  return (
    <div style={{
      position: 'fixed', inset: 0, zIndex: 60, display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(4px)'
    }} onClick={onClose}>
      <div style={{
        background: '#111213', borderRadius: 12, border: '1px solid var(--border-default)',
        width: '100%', maxWidth: 440, margin: '0 16px'
      }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 24px', borderBottom: '1px solid var(--border-subtle)' }}>
          <h3 style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-primary)' }}>领用资产</h3>
          <button onClick={onClose} style={{ background: 'none', border: 'none', color: 'var(--text-quaternary)', cursor: 'pointer', fontSize: 16 }}>✕</button>
        </div>
        <div style={{ padding: 24 }}>
          <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 16 }}>
            资产: <span style={{ color: 'var(--text-primary)', fontWeight: 500 }}>{asset.name}</span>
            <span style={{ marginLeft: 8, fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: 'var(--text-quaternary)' }}>{asset.asset_tag}</span>
          </div>

          {error && (
            <div style={{
              marginBottom: 16, padding: '10px 14px', borderRadius: 6,
              background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.2)',
              color: '#f87171', fontSize: 12
            }}>{error}</div>
          )}

          <div style={{ marginBottom: 14 }}>
            <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
              使用人 <span style={{ color: 'var(--danger)', marginLeft: 2 }}>*</span>
            </label>
            <select value={selectedUser} onChange={e => setSelectedUser(e.target.value)}
              style={{
                width: '100%', padding: '7px 10px', borderRadius: 5,
                border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit', cursor: 'pointer'
              }}>
              <option value="">选择用户...</option>
              {users.map(u => (
                <option key={u.id} value={u.id}>{u.username}</option>
              ))}
            </select>
          </div>

          <div style={{ marginBottom: 14 }}>
            <label style={{ display: 'block', fontSize: 12, fontWeight: 500, color: 'var(--text-secondary)', marginBottom: 5 }}>
              备注
            </label>
            <textarea value={notes} onChange={e => setNotes(e.target.value)}
              rows={3}
              style={{
                width: '100%', padding: '7px 10px', borderRadius: 5,
                border: '1px solid var(--border-default)', background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)', fontSize: 13, outline: 'none', fontFamily: 'inherit',
                resize: 'vertical'
              }} />
          </div>

          <div style={{ display: 'flex', gap: 10, marginTop: 20 }}>
            <button type="button" onClick={onClose}
              style={{
                flex: 1, padding: '8px', borderRadius: 6, border: '1px solid var(--border-default)',
                background: 'transparent', color: 'var(--text-secondary)', fontSize: 13, fontWeight: 500,
                cursor: 'pointer', fontFamily: 'inherit'
              }}>取消</button>
            <button onClick={handleSubmit} disabled={loading || !selectedUser}
              style={{
                flex: 1, padding: '8px', borderRadius: 6, border: 'none',
                background: (loading || !selectedUser) ? 'rgba(94,106,210,0.4)' : 'var(--brand)',
                color: 'white', fontSize: 13, fontWeight: 500, cursor: (loading || !selectedUser) ? 'default' : 'pointer',
                fontFamily: 'inherit'
              }}>{loading ? '提交中...' : '确认领用'}</button>
          </div>
        </div>
      </div>
    </div>
  )
}

// FilterSelect — 统一筛选下拉组件
function FilterSelect({ label, value, onChange, options }: {
  label: string; value: string; onChange: (v: string) => void;
  options: [string, string][]
}) {
  return (
    <select value={value} onChange={e => onChange(e.target.value)}
      style={{
        padding: '7px 10px', borderRadius: 6, border: '1px solid var(--border-default)',
        background: `rgba(255,255,255,${value ? '0.04' : '0.02'})`,
        color: value ? 'var(--text-primary)' : 'var(--text-tertiary)',
        fontSize: 13, outline: 'none', fontFamily: 'inherit', cursor: 'pointer',
        minWidth: 110,
      }}>
      {options.map(([k, v]) => (
        <option key={k} value={k}>{v}</option>
      ))}
    </select>
  )
}
