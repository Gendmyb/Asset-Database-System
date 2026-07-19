// 目录集成管理页 (Wave 3 T9)
// 功能: LDAP 状态卡片 / 手动同步 / 上次同步结果 / 安全组→角色映射 CRUD
import { useState, useEffect, useCallback } from 'react'
import * as directoryApi from '../../api/directory'
import type { GroupMapping, LDAPStatus } from '../../api/directory'

type EditState = { id: string; role: string; data_scope: string; sync_enabled: boolean; group_name: string }
type NewForm = { group_dn: string; group_name: string; role: string; data_scope: string }

const ROLES = ['super_admin', 'admin', 'manager', 'viewer'] as const
const SCOPES = ['inherit', 'self'] as const

export default function DirectoryPage() {
  const [status, setStatus] = useState<LDAPStatus | null>(null)
  const [mappings, setMappings] = useState<GroupMapping[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [syncResult, setSyncResult] = useState<string | null>(null)
  const [editing, setEditing] = useState<EditState | null>(null)
  const [showNew, setShowNew] = useState(false)
  const [newForm, setNewForm] = useState<NewForm>({ group_dn: '', group_name: '', role: 'viewer', data_scope: 'inherit' })

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [s, m] = await Promise.all([directoryApi.getLDAPStatus(), directoryApi.listGroups()])
      setStatus(s)
      setMappings(m)
      setError(null)
    } catch (e: unknown) {
      setError(String(e))
    }
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const handleSync = async () => {
    setSyncing(true)
    setSyncResult(null)
    try {
      // 调用已有同步端点
      const resp = await fetch('/api/v1/admin/ldap/sync', {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('access_token') || ''}` },
      })
      const data = await resp.json()
      setSyncResult(resp.ok ? `同步完成: ${JSON.stringify(data)}` : `同步失败: ${data?.error || resp.status}`)
    } catch (e: unknown) {
      setSyncResult(`请求失败: ${String(e)}`)
    }
    setSyncing(false)
    load() // 刷新状态
  }

  const handleCreate = async () => {
    try {
      await directoryApi.createGroup(newForm)
      setShowNew(false)
      setNewForm({ group_dn: '', group_name: '', role: 'viewer', data_scope: 'inherit' })
      load()
    } catch (e: unknown) { setError(String(e)) }
  }

  const handleUpdate = async (id: string) => {
    if (!editing || editing.id !== id) return
    try {
      await directoryApi.updateGroup(id, {
        role: editing.role,
        data_scope: editing.data_scope,
        sync_enabled: editing.sync_enabled,
        group_name: editing.group_name || undefined,
      })
      setEditing(null)
      load()
    } catch (e: unknown) { setError(String(e)) }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('确定删除此组映射?')) return
    try {
      await directoryApi.deleteGroup(id)
      load()
    } catch (e: unknown) { setError(String(e)) }
  }

  if (loading) return <div style={{ padding: 24, color: 'var(--text-tertiary)' }}>加载中...</div>

  return (
    <div style={{ padding: 24, maxWidth: 960 }}>
      <h2 style={{ fontSize: 18, fontWeight: 600, marginBottom: 16 }}>目录集成</h2>

      {/* LDAP 状态卡片 */}
      <div style={{
        background: 'var(--bg-surface)',
        border: '1px solid var(--border-default)',
        borderRadius: 8,
        padding: 20,
        marginBottom: 24,
      }}>
        <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>LDAP 连接状态</h3>
        <div style={{ display: 'flex', gap: 24, fontSize: 13 }}>
          <StatusLabel label="已启用" ok={status?.enabled} />
          <StatusLabel label="已连通" ok={status?.connected} />
        </div>
        {status?.host && (
          <div style={{ marginTop: 8, fontSize: 12, color: 'var(--text-tertiary)' }}>
            服务器: {status.host}:{status.port}
          </div>
        )}
        <div style={{ marginTop: 16, display: 'flex', gap: 12 }}>
          <button
            onClick={handleSync}
            disabled={syncing || !status?.enabled}
            style={btnStyle}
          >
            {syncing ? '同步中...' : '手动同步'}
          </button>
        </div>
        {syncResult && (
          <div style={{ marginTop: 8, fontSize: 12, color: 'var(--text-secondary)', maxWidth: 500, overflow: 'auto' }}>
            {syncResult}
          </div>
        )}
      </div>

      {error && (
        <div style={{ background: 'var(--red-2)', color: 'var(--red-11)', padding: '8px 12px', borderRadius: 6, marginBottom: 16, fontSize: 13 }}>
          {error}
          <button onClick={() => setError(null)} style={{ marginLeft: 8, background: 'none', border: 'none', cursor: 'pointer' }}>×</button>
        </div>
      )}

      {/* 组映射管理 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <h3 style={{ fontSize: 14, fontWeight: 600 }}>安全组 → 角色映射</h3>
        <button onClick={() => setShowNew(!showNew)} style={{ ...btnStyle, fontSize: 12 }}>
          {showNew ? '取消' : '+ 新建映射'}
        </button>
      </div>

      {/* 新建表单 */}
      {showNew && (
        <div style={{
          background: 'var(--bg-surface)',
          border: '1px solid var(--border-default)',
          borderRadius: 8,
          padding: 16,
          marginBottom: 16,
        }}>
          <InlineField label="组 DN" value={newForm.group_dn} onChange={(v) => setNewForm({ ...newForm, group_dn: v })} placeholder="CN=IT-Admins,OU=Groups,DC=..." />
          <InlineField label="组名(选填)" value={newForm.group_name} onChange={(v) => setNewForm({ ...newForm, group_name: v })} />
          <div style={{ display: 'flex', gap: 12, marginTop: 8 }}>
            <SelectField label="角色" value={newForm.role} options={ROLES} onChange={(v) => setNewForm({ ...newForm, role: v })} />
            <SelectField label="数据范围" value={newForm.data_scope} options={SCOPES} onChange={(v) => setNewForm({ ...newForm, data_scope: v })} />
          </div>
          <button onClick={handleCreate} style={{ ...btnStyle, marginTop: 12 }}>保存</button>
        </div>
      )}

      {/* 映射列表 */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {mappings.map((m) => (
          <div key={m.id} style={{
            background: 'var(--bg-surface)',
            border: '1px solid var(--border-default)',
            borderRadius: 8,
            padding: '12px 16px',
          }}>
            {editing?.id === m.id ? (
              <div>
                <InlineField label="组 DN" value={m.group_dn} onChange={() => {}} disabled />
                <InlineField label="组名" value={editing.group_name} onChange={(v) => setEditing({ ...editing, group_name: v })} />
                <div style={{ display: 'flex', gap: 12, marginTop: 8 }}>
                  <SelectField label="角色" value={editing.role} options={ROLES} onChange={(v) => setEditing({ ...editing, role: v })} />
                  <SelectField label="数据范围" value={editing.data_scope} options={SCOPES} onChange={(v) => setEditing({ ...editing, data_scope: v })} />
                  <label style={{ fontSize: 12, display: 'flex', alignItems: 'center', gap: 4 }}>
                    <input type="checkbox" checked={editing.sync_enabled} onChange={(e) => setEditing({ ...editing, sync_enabled: e.target.checked })} />
                    启用同步
                  </label>
                </div>
                <div style={{ marginTop: 8, display: 'flex', gap: 8 }}>
                  <button onClick={() => handleUpdate(m.id)} style={btnStyle}>保存</button>
                  <button onClick={() => setEditing(null)} style={{ ...btnStyle, background: 'var(--bg-inset)' }}>取消</button>
                </div>
              </div>
            ) : (
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                  <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 2 }}>
                    {m.group_name || m.group_dn}
                    {!m.sync_enabled && <span style={{ marginLeft: 6, fontSize: 11, color: 'var(--red-11)' }}>已禁用</span>}
                  </div>
                  <div style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>{m.group_dn}</div>
                  <div style={{ fontSize: 11, marginTop: 4 }}>
                    <Chip label={m.role} />
                    <Chip label={`scope:${m.data_scope}`} />
                  </div>
                </div>
                <div style={{ display: 'flex', gap: 6 }}>
                  <button onClick={() => setEditing({ id: m.id, role: m.role, data_scope: m.data_scope, sync_enabled: m.sync_enabled, group_name: m.group_name || '' })} style={{ ...btnStyle, fontSize: 11 }}>编辑</button>
                  <button onClick={() => handleDelete(m.id)} style={{ ...btnStyle, fontSize: 11, background: 'var(--red-3)', color: 'var(--red-11)' }}>删除</button>
                </div>
              </div>
            )}
          </div>
        ))}
        {mappings.length === 0 && (
          <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>
            暂无组映射。点击"新建映射"添加 AD 安全组。
          </div>
        )}
      </div>
    </div>
  )
}

function StatusLabel({ label, ok }: { label: string; ok?: boolean }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
      <span style={{
        width: 8, height: 8, borderRadius: '50%',
        background: ok ? 'var(--brand)' : 'var(--text-tertiary)',
      }} />
      <span style={{ color: 'var(--text-secondary)' }}>{label}</span>
    </div>
  )
}

function Chip({ label }: { label: string }) {
  return (
    <span style={{
      display: 'inline-block',
      padding: '1px 6px',
      borderRadius: 4,
      background: 'var(--bg-inset)',
      fontSize: 10,
      marginRight: 4,
      color: 'var(--text-secondary)',
    }}>{label}</span>
  )
}

function InlineField({ label, value, onChange, placeholder, disabled }: {
  label: string; value: string; onChange: (v: string) => void; placeholder?: string; disabled?: boolean
}) {
  return (
    <div style={{ marginBottom: 6 }}>
      <label style={{ fontSize: 11, color: 'var(--text-tertiary)', marginRight: 8 }}>{label}</label>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        style={{
          background: 'var(--bg-inset)',
          border: '1px solid var(--border-default)',
          borderRadius: 4,
          padding: '4px 8px',
          fontSize: 12,
          color: 'var(--text-primary)',
          width: 300,
        }}
      />
    </div>
  )
}

function SelectField({ label, value, options, onChange }: {
  label: string; value: string; options: readonly string[]; onChange: (v: string) => void
}) {
  return (
    <div>
      <label style={{ fontSize: 11, color: 'var(--text-tertiary)', marginRight: 6, display: 'block' }}>{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{
          background: 'var(--bg-inset)',
          border: '1px solid var(--border-default)',
          borderRadius: 4,
          padding: '4px 8px',
          fontSize: 12,
          color: 'var(--text-primary)',
        }}
      >
        {options.map((o) => <option key={o} value={o}>{o}</option>)}
      </select>
    </div>
  )
}

const btnStyle: React.CSSProperties = {
  background: 'var(--brand)',
  color: '#fff',
  border: 'none',
  borderRadius: 6,
  padding: '6px 14px',
  fontSize: 13,
  cursor: 'pointer',
  fontWeight: 500,
}
