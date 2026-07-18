import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import DataTable, { Column } from '../components/ui/DataTable'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import Modal from '../components/ui/Modal'
import ConfirmDialog from '../components/ui/ConfirmDialog'
import Input from '../components/ui/Input'
import Select from '../components/ui/Select'
import Spinner from '../components/ui/Spinner'
import * as stocktakeApi from '../api/stocktake'
import { getApiError } from '../lib/errors'

function fmtDate(d?: string): string {
  if (!d) return '—'
  try {
    return new Date(d).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return d
  }
}

interface CreateFormValues {
  name: string
  scope_location_id?: string
  scope_type_id?: string
}

export default function StocktakesPage() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateFormValues>()

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['stocktakes'],
    queryFn: () => stocktakeApi.listPlans(),
    staleTime: 15000,
  })

  const createMutation = useMutation({
    mutationFn: (values: CreateFormValues) =>
      stocktakeApi.createPlan({
        name: values.name,
        scope_location_id: values.scope_location_id || undefined,
        scope_type_id: values.scope_type_id || undefined,
      }),
    onSuccess: () => {
      toast.success('盘点计划已创建')
      queryClient.invalidateQueries({ queryKey: ['stocktakes'] })
      setCreateOpen(false)
      reset()
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  const startMutation = useMutation({
    mutationFn: (planId: string) => stocktakeApi.startPlan(planId),
    onSuccess: () => {
      toast.success('盘点已开始')
      queryClient.invalidateQueries({ queryKey: ['stocktakes'] })
      setConfirmOpen(null)
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  const rows = data?.data ?? []

  const columns: Column<stocktakeApi.StocktakePlan>[] = [
    {
      key: 'index',
      label: '编号',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          #{row.id?.slice(0, 8) ?? '—'}
        </span>
      ),
    },
    {
      key: 'name',
      label: '名称',
      render: (row) => (
        <span style={{ fontSize: 13, fontWeight: 500 }}>{row.name}</span>
      ),
    },
    {
      key: 'status',
      label: '状态',
      render: (row) => (
        <Badge type="stocktake_status" status={row.status} />
      ),
    },
    {
      key: 'created_at',
      label: '创建时间',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {fmtDate(row.created_at)}
        </span>
      ),
    },
    {
      key: 'actions',
      label: '操作',
      render: (row) => {
        if (row.status === 'draft') {
          return (
            <Button
              variant="primary"
              style={{ fontSize: 12, padding: '4px 10px' }}
              onClick={(e) => {
                e.stopPropagation()
                setConfirmOpen(row.id)
              }}
            >
              开始
            </Button>
          )
        }
        if (row.status === 'in_progress') {
          return (
            <Link
              to={`/stocktakes/${row.id}`}
              style={{
                padding: '4px 10px',
                borderRadius: 6,
                background: 'var(--brand)',
                color: 'white',
                textDecoration: 'none',
                fontSize: 12,
                fontWeight: 500,
                display: 'inline-block',
              }}
            >
              继续
            </Link>
          )
        }
        if (row.status === 'completed') {
          return (
            <Button
              variant="secondary"
              style={{ fontSize: 12, padding: '4px 10px' }}
              onClick={(e) => {
                e.stopPropagation()
                stocktakeApi
                  .getReport(row.id)
                  .then(() => toast.info('报告功能开发中'))
                  .catch((err) => toast.error(getApiError(err)))
              }}
            >
              查看报告
            </Button>
          )
        }
        return (
          <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
            —
          </span>
        )
      },
    },
  ]

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
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
            盘点
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
            管理资产盘点计划，核对资产信息
          </p>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}>
          新建盘点
        </Button>
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

      {/* Loading */}
      {isLoading && (
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            padding: 60,
          }}
        >
          <Spinner size={24} />
        </div>
      )}

      {/* Table */}
      {!isLoading && (
        <DataTable
          columns={columns}
          rows={rows}
          emptyState={{
            title: '暂无盘点计划',
            description: '点击"新建盘点"创建第一个盘点计划',
          }}
        />
      )}

      {/* Create Modal */}
      <Modal
        open={createOpen}
        onClose={() => {
          setCreateOpen(false)
          reset()
        }}
        title="新建盘点计划"
      >
        <form
          onSubmit={handleSubmit((values) => createMutation.mutate(values))}
        >
          <Input
            label="计划名称"
            placeholder="如：2024 年度盘点"
            error={errors.name?.message}
            {...register('name', { required: '请输入计划名称' })}
          />
          <Input
            label="盘查地点 ID"
            placeholder="可选，限定范围"
            {...register('scope_location_id')}
          />
          <Input
            label="盘查类型 ID"
            placeholder="可选，限定范围"
            {...register('scope_type_id')}
          />
          <div
            style={{
              display: 'flex',
              gap: 10,
              justifyContent: 'flex-end',
              marginTop: 8,
            }}
          >
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                setCreateOpen(false)
                reset()
              }}
            >
              取消
            </Button>
            <Button variant="primary" type="submit" loading={createMutation.isPending}>
              创建
            </Button>
          </div>
        </form>
      </Modal>

      {/* Confirm Start */}
      {confirmOpen && (
        <ConfirmDialog
          open
          onClose={() => setConfirmOpen(null)}
          title="开始盘点"
          description="开始后系统将根据盘点范围生成待核对项目列表。确认开始？"
          confirmLabel="开始"
          onConfirm={() => startMutation.mutate(confirmOpen)}
          loading={startMutation.isPending}
        />
      )}
    </div>
  )
}
