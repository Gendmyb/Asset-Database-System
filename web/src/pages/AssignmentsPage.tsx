import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import DataTable, { Column } from '../components/ui/DataTable'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import Spinner from '../components/ui/Spinner'
import * as assignmentsApi from '../api/assignments'
import * as assetsApi from '../api/assets'
import { getApiError } from '../lib/errors'

type TabKey = 'all' | 'permanent' | 'temporary' | 'overdue'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'all', label: '全部' },
  { key: 'permanent', label: '领用中' },
  { key: 'temporary', label: '借用中' },
  { key: 'overdue', label: '已逾期' },
]

function fmtDate(d?: string): string {
  if (!d) return '—'
  try {
    return new Date(d).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  } catch {
    return d
  }
}

function isOverdue(row: assignmentsApi.Assignment): boolean {
  if (row.status === 'returned' || !row.due_date) return false
  return new Date(row.due_date) < new Date()
}

export default function AssignmentsPage() {
  const [tab, setTab] = useState<TabKey>('all')
  const queryClient = useQueryClient()

  const params: assignmentsApi.AssignmentListParams = {}
  if (tab === 'permanent') {
    params.type = 'permanent'
    params.status = 'active'
  } else if (tab === 'temporary') {
    params.type = 'temporary'
    params.status = 'active'
  } else if (tab === 'overdue') {
    params.overdue = true
  }

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['assignments', params],
    queryFn: () => assignmentsApi.list(params),
    staleTime: 15000,
  })

  const releaseMutation = useMutation({
    mutationFn: (id: string) => assetsApi.release(id),
    onSuccess: () => {
      toast.success('归还成功')
      queryClient.invalidateQueries({ queryKey: ['assignments'] })
      queryClient.invalidateQueries({ queryKey: ['assets'] })
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  const rows = data?.data ?? []

  const columns: Column<assignmentsApi.Assignment>[] = [
    {
      key: 'asset_name',
      label: '资产名称',
      render: (row) => (
        <Link
          to={`/assets/${row.asset_id}`}
          style={{
            color: 'var(--brand)',
            textDecoration: 'none',
            fontWeight: 500,
            fontSize: 13,
          }}
        >
          {row.asset_name || row.asset_tag || row.asset_id}
        </Link>
      ),
    },
    {
      key: 'assigned_to_name',
      label: '用户',
      render: (row) => (
        <span style={{ fontSize: 13 }}>
          {row.assigned_to_name || row.assigned_to}
        </span>
      ),
    },
    {
      key: 'assignment_type',
      label: '类型',
      render: (row) => (
        <Badge
          type="assignment"
          status={row.assignment_type || 'temporary'}
        />
      ),
    },
    {
      key: 'created_at',
      label: '领用日期',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {fmtDate(row.created_at)}
        </span>
      ),
    },
    {
      key: 'due_date',
      label: '应还日期',
      render: (row) => {
        if (row.assignment_type !== 'temporary') return <span style={{ color: 'var(--text-quaternary)' }}>—</span>
        const overdue = isOverdue(row)
        return (
          <span
            style={{
              fontSize: 13,
              color: overdue ? '#f87171' : 'var(--text-secondary)',
              fontWeight: overdue ? 600 : 400,
            }}
          >
            {fmtDate(row.due_date)}
          </span>
        )
      },
    },
    {
      key: 'status',
      label: '状态',
      render: (row) => {
        if (row.status === 'returned') {
          return (
            <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
              已归还
            </span>
          )
        }
        if (isOverdue(row)) {
          return (
            <span style={{ fontSize: 12, color: '#f87171', fontWeight: 600 }}>
              已逾期
            </span>
          )
        }
        return (
          <span style={{ fontSize: 12, color: '#4ade80' }}>
            进行中
          </span>
        )
      },
    },
    {
      key: 'actions',
      label: '操作',
      render: (row) => {
        if (row.status === 'returned') return <span style={{ color: 'var(--text-quaternary)', fontSize: 12 }}>—</span>
        return (
          <Button
            variant="secondary"
            onClick={(e) => {
              e.stopPropagation()
              releaseMutation.mutate(row.asset_id)
            }}
            disabled={releaseMutation.isPending}
            style={{ fontSize: 12, padding: '4px 10px' }}
          >
            归还
          </Button>
        )
      },
    },
  ]

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ marginBottom: 24 }}>
        <h1
          style={{
            fontSize: 20,
            fontWeight: 600,
            color: 'var(--text-primary)',
            letterSpacing: '-0.24px',
            margin: '0 0 4px',
          }}
        >
          领用与借用
        </h1>
        <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
          管理所有资产的领用与借用记录
        </p>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 20 }}>
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: '7px 16px',
              borderRadius: 6,
              border: 'none',
              cursor: 'pointer',
              background:
                tab === t.key
                  ? 'var(--brand)'
                  : 'rgba(255,255,255,0.03)',
              color:
                tab === t.key
                  ? '#fff'
                  : 'var(--text-secondary)',
              fontSize: 13,
              fontWeight: tab === t.key ? 600 : 400,
              fontFamily: 'inherit',
              transition: 'background .15s, color .15s',
            }}
          >
            {t.label}
          </button>
        ))}
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
            title: '暂无领用/借用记录',
            description:
              tab !== 'all'
                ? '当前筛选条件下没有匹配的记录'
                : '还没有资产的领用或借用记录',
          }}
        />
      )}
    </div>
  )
}
