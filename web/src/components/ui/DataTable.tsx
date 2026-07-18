import { ReactNode } from 'react'
import Spinner from './Spinner'
import EmptyState from './EmptyState'

export interface Column<T> {
  key: string
  label: string
  render?: (row: T) => ReactNode
}

interface DataTableProps<T> {
  columns: Column<T>[]
  rows: T[]
  onRowClick?: (row: T) => void
  emptyState?: { title: string; description?: string }
  loading?: boolean
}

export default function DataTable<T>({
  columns,
  rows,
  onRowClick,
  emptyState,
  loading = false,
}: DataTableProps<T>) {
  return (
    <div
      style={{
        background: 'var(--bg-surface)',
        borderRadius: 10,
        border: '1px solid var(--border-subtle)',
        overflow: 'hidden',
      }}
    >
      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ borderBottom: '1px solid var(--border-subtle)' }}>
            {columns.map((col) => (
              <th
                key={col.key}
                style={{
                  padding: '10px 16px',
                  textAlign: 'left',
                  fontSize: 11,
                  fontWeight: 500,
                  color: 'var(--text-quaternary)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.5px',
                  position: 'sticky',
                  top: 0,
                  background: 'var(--bg-surface)',
                }}
              >
                {col.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {!loading && rows.length === 0 && (
            <tr>
              <td colSpan={columns.length}>
                <EmptyState
                  title={emptyState?.title || '暂无数据'}
                  description={emptyState?.description}
                />
              </td>
            </tr>
          )}
          {rows.map((row, i) => (
            <tr
              key={((row as any).id as string) || i}
              onClick={() => onRowClick?.(row)}
              style={{
                borderBottom: '1px solid var(--border-subtle)',
                cursor: onRowClick ? 'pointer' : 'default',
                transition: 'background .1s',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = 'rgba(255,255,255,0.02)'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = 'transparent'
              }}
            >
              {columns.map((col) => (
                <td
                  key={col.key}
                  style={{
                    padding: '10px 16px',
                    fontSize: 13,
                    color: 'var(--text-primary)',
                  }}
                >
                  {col.render
                    ? col.render(row)
                    : ((row as any)[col.key] as ReactNode) ?? '—'}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>

      {loading && (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 32 }}>
          <Spinner size={20} />
        </div>
      )}
    </div>
  )
}
