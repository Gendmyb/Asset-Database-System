import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import DataTable, { Column } from '../components/ui/DataTable'
import Modal from '../components/ui/Modal'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import Input from '../components/ui/Input'
import Select from '../components/ui/Select'
import FormField from '../components/ui/FormField'
import ConfirmDialog from '../components/ui/ConfirmDialog'
import Spinner from '../components/ui/Spinner'
import * as maintenanceApi from '../api/maintenance'
import * as assetsApi from '../api/assets'
import { getApiError } from '../lib/errors'
import { useRole } from '../lib/roles'

function fmtDate(d: string) {
  try {
    return new Date(d).toLocaleDateString('zh-CN', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return d
  }
}

type CreateFormData = { asset_id: string; title: string; category: string; description: string }
type CompleteFormData = { resolution: string; cost: string }

export default function MaintenancePage() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [completingTicket, setCompletingTicket] = useState<maintenanceApi.MaintenanceTicket | null>(null)
  const [cancelingTicket, setCancelingTicket] = useState<maintenanceApi.MaintenanceTicket | null>(null)

  const { canManage } = useRole()
  const queryClient = useQueryClient()

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['maintenance', search, statusFilter],
    queryFn: () =>
      maintenanceApi.list({
        search: search || undefined,
        status: statusFilter || undefined,
      }),
  })

  const rows = data?.data ?? []

  const { data: assetsData } = useQuery({
    queryKey: ['assets', 'for-maintenance'],
    queryFn: () => assetsApi.list({ limit: 200 }),
  })
  const assetOptions = (assetsData?.data ?? []).map((a: any) => ({
    value: a.id,
    label: `${a.name}${a.asset_tag ? ` (${a.asset_tag})` : ''}`,
  }))

  const createForm = useForm<CreateFormData>({
    defaultValues: { asset_id: '', title: '', category: 'repair', description: '' },
  })

  const completeForm = useForm<CompleteFormData>({
    defaultValues: { resolution: '', cost: '' },
  })

  const createMutation = useMutation({
    mutationFn: (formData: CreateFormData) =>
      maintenanceApi.create({
        asset_id: formData.asset_id,
        title: formData.title,
        category: formData.category,
        description: formData.description || undefined,
      }),
    onSuccess: () => {
      toast.success('工单创建成功')
      setShowCreate(false)
      createForm.reset()
      queryClient.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const startMutation = useMutation({
    mutationFn: (id: string) => maintenanceApi.start(id),
    onSuccess: () => {
      toast.success('工单已开始处理')
      queryClient.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const completeMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: maintenanceApi.CompleteMaintenanceData }) =>
      maintenanceApi.complete(id, data),
    onSuccess: () => {
      toast.success('工单已完成')
      setCompletingTicket(null)
      completeForm.reset()
      queryClient.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const cancelMutation = useMutation({
    mutationFn: (id: string) => maintenanceApi.cancel(id),
    onSuccess: () => {
      toast.success('工单已取消')
      setCancelingTicket(null)
      queryClient.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const columns: Column<maintenanceApi.MaintenanceTicket>[] = [
    {
      key: 'order_no',
      label: '工单号',
      render: (row) => (
        <span
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 12,
            background: 'rgba(255,255,255,0.05)',
            padding: '2px 8px',
            borderRadius: 4,
            color: 'var(--text-secondary)',
          }}
        >
          {row.order_no}
        </span>
      ),
    },
    {
      key: 'asset',
      label: '资产',
      render: (row) => (
        <span style={{ fontWeight: 500, color: 'var(--text-primary)' }}>
          {row.asset_name || row.asset_tag || row.asset_id || '—'}
        </span>
      ),
    },
    {
      key: 'title',
      label: '标题',
      render: (row) => <span>{row.title}</span>,
    },
    {
      key: 'category',
      label: '类别',
      render: (row) => <Badge type="maintenance_category" status={row.category} />,
    },
    {
      key: 'status',
      label: '状态',
      render: (row) => <Badge type="maintenance_status" status={row.status} />,
    },
    {
      key: 'created_at',
      label: '时间',
      render: (row) => (
        <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>{fmtDate(row.created_at)}</span>
      ),
    },
    {
      key: 'actions',
      label: '操作',
      render: (row) => {
        if (row.status === 'completed' || row.status === 'canceled') {
          return <span style={{ color: 'var(--text-quaternary)', fontSize: 12 }}>—</span>
        }
        if (!canManage) {
          return <span style={{ color: 'var(--text-quaternary)', fontSize: 12 }}>—</span>
        }
        return (
          <div style={{ display: 'flex', gap: 6 }}>
            {row.status === 'open' && (
              <Button
                variant="secondary"
                style={{ fontSize: 12, padding: '4px 10px' }}
                onClick={(e) => {
                  e.stopPropagation()
                  startMutation.mutate(row.id)
                }}
                loading={startMutation.isPending}
              >
                开始
              </Button>
            )}
            {row.status === 'in_progress' && (
              <>
                <Button
                  variant="primary"
                  style={{ fontSize: 12, padding: '4px 10px' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    completeForm.reset()
                    setCompletingTicket(row)
                  }}
                >
                  完成
                </Button>
                <Button
                  variant="ghost"
                  style={{ fontSize: 12, padding: '4px 10px', color: '#f87171' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    setCancelingTicket(row)
                  }}
                >
                  取消
                </Button>
              </>
            )}
          </div>
        )
      },
    },
  ]

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
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
            维修保养
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>管理资产维修与保养工单</p>
        </div>
        {canManage && (
          <Button
            onClick={() => {
              createForm.reset()
              setShowCreate(true)
            }}
          >
            + 新建工单
          </Button>
        )}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 20 }}>
        <div style={{ flex: 1, maxWidth: 320 }}>
          <Input
            placeholder="搜索工单号、标题..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div style={{ minWidth: 160 }}>
          <Select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            options={[
              { value: '', label: '全部状态' },
              { value: 'open', label: '待处理' },
              { value: 'in_progress', label: '处理中' },
              { value: 'completed', label: '已完成' },
              { value: 'canceled', label: '已取消' },
            ]}
          />
        </div>
      </div>

      {/* Error */}
      {isError && (
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 8,
            background: 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.2)',
            color: '#f87171',
            fontSize: 13,
            marginBottom: 16,
          }}
        >
          {getApiError(error)}
        </div>
      )}

      {/* Table */}
      {isLoading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 60 }}>
          <Spinner size={24} />
        </div>
      ) : (
        <DataTable
          columns={columns}
          rows={rows}
          emptyState={{
            title: '暂无工单',
            description:
              search || statusFilter
                ? '当前筛选条件下没有匹配的工单'
                : '还没有维修保养工单，点击上方按钮创建',
          }}
        />
      )}

      {/* Create Modal */}
      <Modal open={showCreate} onClose={() => setShowCreate(false)} title="新建维修保养工单" width="480px">
        <form onSubmit={createForm.handleSubmit((data) => createMutation.mutate(data))}>
          <FormField label="资产" required>
            <select
              {...createForm.register('asset_id', { required: true })}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                cursor: 'pointer',
              }}
            >
              <option value="">选择资产...</option>
              {assetOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
          <Input
            label="标题"
            placeholder="例如：更换显示器屏幕"
            {...createForm.register('title', { required: true })}
          />
          <Select
            label="类别"
            {...createForm.register('category')}
            options={[
              { value: 'repair', label: '维修' },
              { value: 'upkeep', label: '保养' },
            ]}
          />
          <FormField label="描述">
            <textarea
              {...createForm.register('description')}
              rows={4}
              placeholder="详细描述故障或保养需求..."
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                resize: 'vertical',
              }}
            />
          </FormField>
          <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 8 }}>
            <Button variant="secondary" type="button" onClick={() => setShowCreate(false)}>
              取消
            </Button>
            <Button type="submit" loading={createMutation.isPending}>
              创建工单
            </Button>
          </div>
        </form>
      </Modal>

      {/* Complete Modal */}
      <Modal
        open={!!completingTicket}
        onClose={() => setCompletingTicket(null)}
        title="完成工单"
        width="480px"
      >
        <form
          onSubmit={completeForm.handleSubmit((data) => {
            if (!completingTicket) return
            completeMutation.mutate({
              id: completingTicket.id,
              data: {
                resolution: data.resolution || undefined,
                cost: data.cost ? Number(data.cost) : undefined,
              },
            })
          })}
        >
          <FormField label="处理结果">
            <textarea
              {...completeForm.register('resolution')}
              rows={3}
              placeholder="描述维修/保养处理结果..."
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                resize: 'vertical',
              }}
            />
          </FormField>
          <Input label="费用" type="number" placeholder="0.00" {...completeForm.register('cost')} />
          <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end', marginTop: 8 }}>
            <Button variant="secondary" type="button" onClick={() => setCompletingTicket(null)}>
              取消
            </Button>
            <Button type="submit" loading={completeMutation.isPending}>
              确认完成
            </Button>
          </div>
        </form>
      </Modal>

      {/* Cancel Confirm */}
      <ConfirmDialog
        open={!!cancelingTicket}
        onClose={() => setCancelingTicket(null)}
        title="取消工单"
        description={`确认取消工单 ${cancelingTicket?.order_no || ''} 吗？此操作不可撤销。`}
        confirmLabel="确认取消"
        danger
        onConfirm={() => {
          if (cancelingTicket) cancelMutation.mutate(cancelingTicket.id)
        }}
        loading={cancelMutation.isPending}
      />
    </div>
  )
}
