import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import DataTable from '../ui/DataTable'
import Badge from '../ui/Badge'
import AssetFilters from './AssetFilters'
import type { Column } from '../ui/DataTable'
import * as assetsApi from '../../api/assets'
import type { Asset } from '../../api/assets'

const LIFECYCLE_LABELS: Record<string, string> = {
  deployment: '部署中',
  utilization: '使用中',
  maintenance: '维护中',
  retirement: '已退役',
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

const columns: Column<Asset>[] = [
  {
    key: 'asset_tag',
    label: '编号',
    render: (row: Asset) => (
      <span
        style={{
          fontFamily: 'JetBrains Mono, monospace',
          fontSize: 12,
          background: 'rgba(0,0,0,0.05)',
          color: 'var(--text-secondary)',
          padding: '2px 8px',
          borderRadius: 4,
        }}
      >
        {row.asset_tag}
      </span>
    ),
  },
  {
    key: 'name',
    label: '名称',
    render: (row: Asset) => (
      <span style={{ fontWeight: 500, color: 'var(--text-primary)' }}>
        {row.name}
      </span>
    ),
  },
  {
    key: 'manufacturer',
    label: '制造商',
    render: (row: Asset) => (
      <span style={{ color: 'var(--text-tertiary)' }}>
        {row.manufacturer || '—'}
      </span>
    ),
  },
  {
    key: 'model',
    label: '型号',
    render: (row: Asset) => (
      <span style={{ color: 'var(--text-tertiary)' }}>
        {row.model || '—'}
      </span>
    ),
  },
  {
    key: 'status',
    label: '状态',
    render: (row: Asset) => <Badge status={row.status} />,
  },
  {
    key: 'lifecycle_state',
    label: '生命周期',
    render: (row: Asset) => (
      <span style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
        {LIFECYCLE_LABELS[row.lifecycle_state] || row.lifecycle_state}
      </span>
    ),
  },
  {
    key: 'updated_at',
    label: '更新时间',
    render: (row: Asset) => (
      <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
        {fmtDate(row.updated_at)}
      </span>
    ),
  },
]

interface AssetTableProps {
  onSelect: (asset: Asset) => void
  search: string
  onSearchChange: (v: string) => void
  status: string
  onStatusChange: (v: string) => void
  lifecycle: string
  onLifecycleChange: (v: string) => void
  manufacturer: string
  onManufacturerChange: (v: string) => void
}

export default function AssetTable({
  onSelect,
  search,
  onSearchChange,
  status,
  onStatusChange,
  lifecycle,
  onLifecycleChange,
  manufacturer,
  onManufacturerChange,
}: AssetTableProps) {
  const [allRows, setAllRows] = useState<Asset[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)

  const queryKey = ['assets', search, status, lifecycle, manufacturer]

  const { data, isLoading, isError } = useQuery({
    queryKey,
    queryFn: () =>
      assetsApi.list({
        search: search || undefined,
        status: status || undefined,
        lifecycle: lifecycle || undefined,
        manufacturer: manufacturer || undefined,
        limit: 20,
      }),
  })

  useEffect(() => {
    if (data) {
      setAllRows(data.data)
      setCursor(data.pagination.next_cursor)
      setHasMore(data.pagination.has_more)
    }
  }, [data])

  const loadMore = async () => {
    if (!hasMore || loadingMore) return
    setLoadingMore(true)
    try {
      const more = await assetsApi.list({
        search: search || undefined,
        status: status || undefined,
        lifecycle: lifecycle || undefined,
        manufacturer: manufacturer || undefined,
        cursor: cursor!,
        limit: 20,
      })
      setAllRows((prev) => [...prev, ...more.data])
      setCursor(more.pagination.next_cursor)
      setHasMore(more.pagination.has_more)
    } finally {
      setLoadingMore(false)
    }
  }

  if (isError) {
    return (
      <div
        style={{
          padding: 40,
          textAlign: 'center',
          color: 'var(--text-tertiary)',
        }}
      >
        加载失败，请重试
      </div>
    )
  }

  return (
    <div>
      <AssetFilters
        search={search}
        onSearchChange={onSearchChange}
        status={status}
        onStatusChange={onStatusChange}
        lifecycle={lifecycle}
        onLifecycleChange={onLifecycleChange}
        manufacturer={manufacturer}
        onManufacturerChange={onManufacturerChange}
      />

      <DataTable
        columns={columns}
        rows={allRows}
        onRowClick={onSelect}
        loading={isLoading}
        emptyState={{
          title: '暂无资产',
          description: 'Create your first asset to get started',
        }}
      />

      {hasMore && !isLoading && (
        <button
          onClick={loadMore}
          disabled={loadingMore}
          style={{
            width: '100%',
            padding: '10px',
            border: 'none',
            background: 'var(--bg-surface)',
            cursor: 'pointer',
            color: 'var(--text-tertiary)',
            fontSize: 13,
            fontFamily: 'inherit',
            borderRadius: '0 0 10px 10px',
            borderTop: '1px solid var(--border-subtle)',
            marginTop: -1,
          }}
        >
          {loadingMore ? '加载中...' : '加载更多...'}
        </button>
      )}
    </div>
  )
}
