import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import Drawer from '../ui/Drawer'
import Badge from '../ui/Badge'
import Button from '../ui/Button'
import Input from '../ui/Input'
import Select from '../ui/Select'
import * as assetsApi from '../../api/assets'
import * as assignmentsApi from '../../api/assignments'
import * as usersApi from '../../api/users'
import { getApiError } from '../../lib/errors'
import { toast as sonnerToast } from 'sonner'

const LIFECYCLE_LABELS: Record<string, string> = {
  procurement: '采购中',
  deployment: '部署中',
  utilization: '使用中',
  maintenance: '维护中',
  retirement: '已退役',
}

const ALLOWED_TRANSITIONS: Record<string, string[]> = {
  procurement: ['deployment'],
  deployment: ['utilization', 'maintenance'],
  utilization: ['maintenance', 'retirement'],
  maintenance: ['utilization', 'retirement'],
  retirement: [],
}

function fmtDate(d: string) {
  try {
    return new Date(d).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
  } catch {
    return d
  }
}

interface AssetDetailPanelProps {
  asset: assetsApi.Asset
  onClose: () => void
  onRefresh: (id: string) => void
  onAssign: (asset: assetsApi.Asset) => void
  onBorrow: (asset: assetsApi.Asset) => void
  onRelease: (id: string) => void
}

export default function AssetDetailPanel({
  asset,
  onClose,
  onRefresh,
  onAssign,
  onBorrow,
  onRelease,
}: AssetDetailPanelProps) {
  const [editMode, setEditMode] = useState(false)
  const [error, setError] = useState('')
  const [assignedUser, setAssignedUser] = useState<string | null>(null)
  const [form, setForm] = useState({
    name: asset.name,
    manufacturer: asset.manufacturer || '',
    model: asset.model || '',
    serial_number: asset.serial_number || '',
    status: asset.status,
  })

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

  // Fetch assigned user
  useEffect(() => {
    let cancelled = false
    if (asset.status !== 'assigned') {
      setAssignedUser(null)
      return
    }
    assignmentsApi.getByAsset(asset.id)
      .then((assignment) => {
        if (cancelled) return
        const a = assignment as any
        const userId = a?.assigned_to
        if (userId) {
          usersApi.get(userId)
            .then((userData) => {
              if (!cancelled) {
                const name =
                  (userData as any)?.username || userId
                setAssignedUser(name)
              }
            })
            .catch(() => {
              if (!cancelled) setAssignedUser(userId)
            })
        } else {
          setAssignedUser(null)
        }
      })
      .catch(() => {
        if (!cancelled) setAssignedUser(null)
      })
    return () => {
      cancelled = true
    }
  }, [asset.id, asset.status])

  const queryClient = useQueryClient()

  const saveMutation = useMutation({
    mutationFn: () =>
      assetsApi.update(asset.id, form, asset.version),
    onSuccess: () => {
      setEditMode(false)
      onRefresh(asset.id)
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      sonnerToast.success('保存成功')
    },
    onError: (err) => {
      setError(getApiError(err))
    },
  })

  const transitionMutation = useMutation({
    mutationFn: (target: string) =>
      assetsApi.transition(asset.id, target),
    onSuccess: () => {
      onRefresh(asset.id)
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      sonnerToast.success('状态转换成功')
    },
    onError: (err) => {
      setError(getApiError(err))
    },
  })

  const releasesMutation = useMutation({
    mutationFn: () => assetsApi.release(asset.id),
    onSuccess: () => {
      onRefresh(asset.id)
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      sonnerToast.success('归还成功')
    },
    onError: (err) => {
      setError(getApiError(err))
    },
  })

  const transitions = ALLOWED_TRANSITIONS[asset.lifecycle_state] || []
  const managedBy =
    (asset.properties?.managed_by as string) || null

  return (
    <Drawer open={true} onClose={onClose} title="资产详情" width="400px">
      {/* Icon + Tag */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <div
          style={{
            width: 56,
            height: 56,
            background: 'rgba(94,106,210,0.1)',
            borderRadius: 14,
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            marginBottom: 12,
          }}
        >
          <svg
            width="24"
            height="24"
            viewBox="0 0 24 24"
            fill="none"
            stroke="var(--brand)"
            strokeWidth="1.5"
          >
            <path d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
        </div>
        <div
          style={{
            fontSize: 16,
            fontWeight: 600,
            color: 'var(--text-primary)',
          }}
        >
          {asset.name}
        </div>
        <div
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 12,
            color: 'var(--text-tertiary)',
            marginTop: 4,
          }}
        >
          {asset.asset_tag}{' '}
          <span style={{ color: 'var(--text-quaternary)' }}>
            · v{asset.version}
          </span>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div
          style={{
            marginBottom: 16,
            padding: '10px 14px',
            borderRadius: 6,
            background: 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.2)',
            color: '#f87171',
            fontSize: 12,
          }}
        >
          {error}
        </div>
      )}

      {/* Edit mode */}
      {editMode ? (
        <div style={{ marginBottom: 20 }}>
          <Input
            label="名称"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <Input
            label="制造商"
            value={form.manufacturer}
            onChange={(e) =>
              setForm({ ...form, manufacturer: e.target.value })
            }
          />
          <Input
            label="型号"
            value={form.model}
            onChange={(e) => setForm({ ...form, model: e.target.value })}
          />
          <Input
            label="序列号"
            value={form.serial_number}
            onChange={(e) =>
              setForm({ ...form, serial_number: e.target.value })
            }
          />
          <Select
            label="状态"
            value={form.status}
            onChange={(e) =>
              setForm({ ...form, status: e.target.value })
            }
            options={[
              { value: 'available', label: '可用' },
              { value: 'assigned', label: '已领用' },
              { value: 'maintenance', label: '维护中' },
            ]}
          />
          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <Button
              variant="secondary"
              onClick={() => {
                setEditMode(false)
                setForm({
                  name: asset.name,
                  manufacturer: asset.manufacturer || '',
                  model: asset.model || '',
                  serial_number: asset.serial_number || '',
                  status: asset.status,
                })
                setError('')
              }}
            >
              取消
            </Button>
            <Button
              onClick={() => saveMutation.mutate()}
              loading={saveMutation.isPending}
            >
              保存
            </Button>
          </div>
        </div>
      ) : (
        <>
          {[
            ['制造商', asset.manufacturer],
            ['型号', asset.model],
            ['序列号', asset.serial_number],
            ['状态', asset.status],
            ['使用人', assignedUser],
            ['管理人', managedBy],
          ].map(([k, v]) => (
            <div
              key={k as string}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                padding: '10px 0',
                borderBottom: '1px solid var(--border-subtle)',
              }}
            >
              <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>
                {k}
              </span>
              <span
                style={{
                  fontSize: 12,
                  fontWeight: 500,
                  color: 'var(--text-primary)',
                }}
              >
                {k === '状态' ? (
                  <Badge status={v as string} />
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

          <div style={{ marginTop: 12 }}>
            <Button
              variant="secondary"
              onClick={() => setEditMode(true)}
              style={{ width: '100%' }}
            >
              编辑
            </Button>
          </div>
        </>
      )}

      {/* Lifecycle Section */}
      <div style={{ marginTop: 24 }}>
        <h4
          style={{
            fontSize: 12,
            fontWeight: 600,
            color: 'var(--text-secondary)',
            textTransform: 'uppercase',
            letterSpacing: '0.5px',
            marginBottom: 12,
          }}
        >
          生命周期
        </h4>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            marginBottom: 16,
            padding: '10px 14px',
            borderRadius: 8,
            background: 'rgba(94,106,210,0.08)',
            border: '1px solid rgba(94,106,210,0.15)',
          }}
        >
          <div
            style={{
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: 'var(--brand)',
            }}
          />
          <span
            style={{
              fontSize: 13,
              fontWeight: 600,
              color: 'var(--brand)',
            }}
          >
            {LIFECYCLE_LABELS[asset.lifecycle_state] ||
              asset.lifecycle_state}
          </span>
        </div>

        {transitions.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div
              style={{
                fontSize: 11,
                color: 'var(--text-quaternary)',
                marginBottom: 2,
              }}
            >
              可转换状态：
            </div>
            {transitions.map((target) => (
              <button
                key={target}
                onClick={() => transitionMutation.mutate(target)}
                disabled={transitionMutation.isPending}
                style={{
                  width: '100%',
                  padding: '8px 14px',
                  borderRadius: 6,
                  cursor: transitionMutation.isPending
                    ? 'default'
                    : 'pointer',
                  background: 'rgba(255,255,255,0.03)',
                  border: '1px solid var(--border-default)',
                  color: 'var(--text-secondary)',
                  fontSize: 13,
                  fontWeight: 500,
                  fontFamily: 'inherit',
                  textAlign: 'left',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                }}
              >
                <span>
                  → {LIFECYCLE_LABELS[target] || target}
                </span>
                <svg
                  width="14"
                  height="14"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path d="M5 12h14M12 5l7 7-7 7" />
                </svg>
              </button>
            ))}
          </div>
        )}

        {transitions.length === 0 && (
          <div
            style={{
              fontSize: 12,
              color: 'var(--text-quaternary)',
              padding: '8px 0',
            }}
          >
            终态，不可转换
          </div>
        )}
      </div>

      {/* Actions: Assign / Borrow / Release */}
      <div style={{ marginTop: 24 }}>
        {asset.status === 'available' && (
          <div style={{ display: 'flex', gap: 8 }}>
            <Button
              onClick={() => onAssign(asset)}
              style={{ flex: 1 }}
            >
              领用
            </Button>
            <Button
              onClick={() => onBorrow(asset)}
              variant="secondary"
              style={{ flex: 1 }}
            >
              借用
            </Button>
          </div>
        )}
        {asset.status === 'assigned' && (
          <Button
            onClick={() => releasesMutation.mutate()}
            loading={releasesMutation.isPending}
            variant="primary"
            style={{ width: '100%' }}
          >
            归还
          </Button>
        )}
      </div>

      {/* Meta info */}
      <div
        style={{
          marginTop: 24,
          paddingTop: 16,
          borderTop: '1px solid var(--border-subtle)',
        }}
      >
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            padding: '6px 0',
          }}
        >
          <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
            创建时间
          </span>
          <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
            {fmtDate(asset.created_at)}
          </span>
        </div>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            padding: '6px 0',
          }}
        >
          <span style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
            最后更新
          </span>
          <span style={{ fontSize: 11, color: 'var(--text-tertiary)' }}>
            {fmtDate(asset.updated_at)}
          </span>
        </div>
      </div>
    </Drawer>
  )
}
