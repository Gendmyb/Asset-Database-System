import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import DataTable from '../../components/ui/DataTable'
import type { Column } from '../../components/ui/DataTable'
import Button from '../../components/ui/Button'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Input from '../../components/ui/Input'
import FormField from '../../components/ui/FormField'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import Spinner from '../../components/ui/Spinner'
import * as webhooksApi from '../../api/webhooks'
import type { WebhookEndpoint } from '../../api/webhooks'
import { getApiError } from '../../lib/errors'

// 可订阅事件 (与后端 event/bus.go 常量一致)
const EVENT_OPTIONS: { value: string; label: string }[] = [
  { value: '*', label: '全部事件 (*)' },
  { value: 'asset.created', label: '资产创建' },
  { value: 'asset.updated', label: '资产更新' },
  { value: 'asset.deleted', label: '资产删除' },
  { value: 'asset.assigned', label: '资产领用' },
  { value: 'asset.released', label: '资产归还' },
  { value: 'asset.transferred', label: '资产转移' },
  { value: 'asset.borrowed', label: '资产借用' },
  { value: 'asset.lifecycle_changed', label: '生命周期变更' },
]

function fmtDate(d?: string): string {
  if (!d) return '—'
  try {
    return new Date(d).toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return d
  }
}

interface EndpointForm {
  url: string
  secret: string
  events: string[]
  active: boolean
}

const EMPTY_FORM: EndpointForm = {
  url: '',
  secret: '',
  events: ['*'],
  active: true,
}

export default function Webhooks() {
  const queryClient = useQueryClient()
  const [showModal, setShowModal] = useState(false)
  const [editing, setEditing] = useState<WebhookEndpoint | null>(null)
  const [form, setForm] = useState<EndpointForm>(EMPTY_FORM)
  const [deleteTarget, setDeleteTarget] = useState<WebhookEndpoint | null>(null)
  const [deliveriesFor, setDeliveriesFor] = useState<WebhookEndpoint | null>(null)

  const { data: endpoints, isLoading } = useQuery({
    queryKey: ['admin', 'webhooks'],
    queryFn: () => webhooksApi.list(),
  })

  const list = Array.isArray(endpoints) ? endpoints : []

  const createMutation = useMutation({
    mutationFn: (data: EndpointForm) =>
      webhooksApi.create({
        url: data.url,
        secret: data.secret || undefined,
        events: data.events,
        active: data.active,
      }),
    onSuccess: () => {
      toast.success('Webhook 已创建')
      queryClient.invalidateQueries({ queryKey: ['admin', 'webhooks'] })
      closeModal()
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: EndpointForm }) =>
      webhooksApi.update(id, {
        url: data.url,
        events: data.events,
        active: data.active,
      }),
    onSuccess: () => {
      toast.success('Webhook 已更新')
      queryClient.invalidateQueries({ queryKey: ['admin', 'webhooks'] })
      closeModal()
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      webhooksApi.update(id, { active }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'webhooks'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => webhooksApi.remove(id),
    onSuccess: () => {
      toast.success('Webhook 已删除')
      queryClient.invalidateQueries({ queryKey: ['admin', 'webhooks'] })
      setDeleteTarget(null)
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const openCreate = () => {
    setEditing(null)
    setForm(EMPTY_FORM)
    setShowModal(true)
  }

  const openEdit = (row: WebhookEndpoint) => {
    setEditing(row)
    setForm({
      url: row.url,
      secret: '',
      events: row.events?.length ? row.events : ['*'],
      active: row.active,
    })
    setShowModal(true)
  }

  const closeModal = () => {
    setShowModal(false)
    setEditing(null)
    setForm(EMPTY_FORM)
  }

  const toggleEvent = (value: string) => {
    setForm((prev) => {
      // 选 "*" 清空其它; 选具体事件移除 "*"
      if (value === '*') {
        return prev.events.includes('*') ? { ...prev, events: [] } : { ...prev, events: ['*'] }
      }
      const withoutStar = prev.events.filter((e) => e !== '*')
      const has = withoutStar.includes(value)
      return {
        ...prev,
        events: has ? withoutStar.filter((e) => e !== value) : [...withoutStar, value],
      }
    })
  }

  const submit = () => {
    if (!form.url.trim()) {
      toast.error('请填写回调 URL')
      return
    }
    if (form.events.length === 0) {
      toast.error('请至少选择一个事件')
      return
    }
    if (editing) {
      updateMutation.mutate({ id: editing.id, data: form })
    } else {
      createMutation.mutate(form)
    }
  }

  const columns: Column<WebhookEndpoint>[] = [
    {
      key: 'url',
      label: '回调 URL',
      render: (row) => (
        <span
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 12,
            color: 'var(--text-secondary)',
            wordBreak: 'break-all',
          }}
        >
          {row.url}
        </span>
      ),
    },
    {
      key: 'events',
      label: '订阅事件',
      render: (row) => (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {(row.events || []).map((ev) => (
            <span
              key={ev}
              style={{
                fontSize: 11,
                padding: '2px 6px',
                borderRadius: 4,
                background: 'rgba(94,106,210,0.12)',
                color: 'var(--brand)',
              }}
            >
              {ev}
            </span>
          ))}
        </div>
      ),
    },
    {
      key: 'active',
      label: '状态',
      render: (row) => (
        <button
          onClick={(e) => {
            e.stopPropagation()
            toggleMutation.mutate({ id: row.id, active: !row.active })
          }}
          title="点击切换启用状态"
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            padding: 0,
            fontFamily: 'inherit',
          }}
        >
          <Badge status={row.active ? 'available' : 'retired'} />
        </button>
      ),
    },
    {
      key: 'updated_at',
      label: '更新时间',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
          {fmtDate(row.updated_at)}
        </span>
      ),
    },
    {
      key: 'actions',
      label: '操作',
      render: (row) => (
        <div style={{ display: 'flex', gap: 6 }}>
          <Button
            variant="ghost"
            style={{ fontSize: 12, padding: '4px 8px' }}
            onClick={(e) => {
              e.stopPropagation()
              setDeliveriesFor(row)
            }}
          >
            投递记录
          </Button>
          <Button
            variant="ghost"
            style={{ fontSize: 12, padding: '4px 8px' }}
            onClick={(e) => {
              e.stopPropagation()
              openEdit(row)
            }}
          >
            编辑
          </Button>
          <Button
            variant="ghost"
            style={{ fontSize: 12, padding: '4px 8px', color: '#dc2626' }}
            onClick={(e) => {
              e.stopPropagation()
              setDeleteTarget(row)
            }}
          >
            删除
          </Button>
        </div>
      ),
    },
  ]

  const saving = createMutation.isPending || updateMutation.isPending

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: 24,
        }}
      >
        <div>
          <h1
            style={{
              fontSize: 20,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.24px',
              margin: '0 0 4px',
            }}
          >
            Webhooks
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
            配置事件回调端点，资产变更将推送至订阅的 URL（仅 HTTPS）
          </p>
        </div>
        <Button onClick={openCreate}>+ 新建 Webhook</Button>
      </div>

      <DataTable
        columns={columns}
        rows={list}
        loading={isLoading}
        emptyState={{
          title: '暂无 Webhook',
          description: '创建一个 Webhook 端点以接收资产事件推送',
        }}
      />

      {/* Create / Edit Modal */}
      <Modal
        open={showModal}
        onClose={closeModal}
        title={editing ? '编辑 Webhook' : '新建 Webhook'}
        width="520px"
      >
        <Input
          label="回调 URL"
          placeholder="https://example.com/hooks/asset"
          value={form.url}
          onChange={(e) => setForm({ ...form, url: e.target.value })}
        />
        {!editing && (
          <Input
            label="签名密钥 (可选)"
            placeholder="用于 HMAC 校验，留空则不签名"
            value={form.secret}
            onChange={(e) => setForm({ ...form, secret: e.target.value })}
          />
        )}
        <FormField label="订阅事件" required>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 4 }}>
            {EVENT_OPTIONS.map((opt) => {
              const checked = form.events.includes(opt.value)
              return (
                <label
                  key={opt.value}
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 6,
                    padding: '5px 10px',
                    borderRadius: 5,
                    border: `1px solid ${checked ? 'var(--brand)' : 'var(--border-default)'}`,
                    background: checked ? 'rgba(94,106,210,0.1)' : 'transparent',
                    fontSize: 12,
                    color: 'var(--text-secondary)',
                    cursor: 'pointer',
                    fontFamily: 'inherit',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggleEvent(opt.value)}
                    style={{ cursor: 'pointer' }}
                  />
                  {opt.label}
                </label>
              )
            })}
          </div>
        </FormField>
        <FormField label="启用">
          <label
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 13,
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              fontFamily: 'inherit',
            }}
          >
            <input
              type="checkbox"
              checked={form.active}
              onChange={(e) => setForm({ ...form, active: e.target.checked })}
              style={{ cursor: 'pointer' }}
            />
            创建后立即启用
          </label>
        </FormField>
        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 20 }}>
          <Button variant="secondary" onClick={closeModal} disabled={saving}>
            取消
          </Button>
          <Button onClick={submit} loading={saving}>
            {editing ? '保存' : '创建'}
          </Button>
        </div>
      </Modal>

      {/* Delete Confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="删除 Webhook"
        description="确认删除此 Webhook 端点？该操作不可逆，历史投递记录将保留。"
        confirmLabel="确认删除"
        danger
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        loading={deleteMutation.isPending}
      />

      {/* Deliveries Modal */}
      {deliveriesFor && (
        <DeliveriesModal
          endpoint={deliveriesFor}
          onClose={() => setDeliveriesFor(null)}
        />
      )}
    </div>
  )
}

function DeliveriesModal({
  endpoint,
  onClose,
}: {
  endpoint: WebhookEndpoint
  onClose: () => void
}) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['admin', 'webhooks', endpoint.id, 'deliveries'],
    queryFn: () => webhooksApi.listDeliveries(endpoint.id),
    enabled: !!endpoint.id,
  })

  const deliveries = Array.isArray(data) ? data : []

  return (
    <Modal open={true} onClose={onClose} title="投递记录" width="640px">
      <div
        style={{
          marginBottom: 12,
          padding: '8px 10px',
          borderRadius: 5,
          background: 'rgba(0,0,0,0.02)',
          border: '1px solid var(--border-subtle)',
        }}
      >
        <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>端点：</span>
        <span
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 12,
            color: 'var(--text-secondary)',
            wordBreak: 'break-all',
          }}
        >
          {endpoint.url}
        </span>
      </div>

      {isLoading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
          <Spinner />
        </div>
      ) : isError ? (
        <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>
          加载失败
        </div>
      ) : deliveries.length === 0 ? (
        <div style={{ padding: 24, textAlign: 'center', color: 'var(--text-quaternary)', fontSize: 13 }}>
          暂无投递记录
        </div>
      ) : (
        <div style={{ maxHeight: 360, overflow: 'auto' }}>
          {deliveries.map((d) => (
            <div
              key={d.id}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '8px 0',
                borderBottom: '1px solid var(--border-subtle)',
                fontSize: 12,
              }}
            >
              <Badge status={d.status === 'success' ? 'available' : 'retired'} />
              <span style={{ color: 'var(--text-secondary)', minWidth: 130 }}>
                {d.event_type}
              </span>
              <span style={{ color: 'var(--text-quaternary)' }}>
                {d.status_code ?? '—'} · {d.attempts} 次
              </span>
              {d.last_error && (
                <span
                  style={{ color: '#dc2626', marginLeft: 'auto', wordBreak: 'break-all' }}
                  title={d.last_error}
                >
                  {d.last_error}
                </span>
              )}
              <span style={{ color: 'var(--text-quaternary)', marginLeft: 'auto' }}>
                {fmtDate(d.created_at)}
              </span>
            </div>
          ))}
        </div>
      )}

      <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
        <Button variant="secondary" onClick={onClose}>
          关闭
        </Button>
      </div>
    </Modal>
  )
}
