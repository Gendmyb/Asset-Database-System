import { useQuery, useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import Button from '../components/ui/Button'
import DataTable, { Column } from '../components/ui/DataTable'
import * as reportsApi from '../api/reports'
import * as assetsApi from '../api/assets'
import { downloadBlob } from '../lib/download'
import { getApiError } from '../lib/errors'
import { useAuthStore } from '../store/authStore'

const KPI_CARD: React.CSSProperties = {
  background: 'var(--bg-surface)',
  borderRadius: 10,
  padding: '20px 24px',
  border: '1px solid var(--border-subtle)',
}

function KpiCard({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div style={{ ...KPI_CARD, borderLeft: `3px solid ${color}` }}>
      <div style={{ fontSize: 12, color: 'var(--text-tertiary)', fontWeight: 500, marginBottom: 8 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.5px' }}>{value}</div>
    </div>
  )
}

export default function ReportsPage() {
  const role = useAuthStore((s) => s.user?.role)
  const isAdmin = role === 'admin' || role === 'super_admin'

  const { data: summaryData, isLoading: summaryLoading } = useQuery({
    queryKey: ['reports', 'summary'],
    queryFn: () => reportsApi.summary(),
  })

  const { data: depreciationData, isLoading: depreciationLoading } = useQuery({
    queryKey: ['reports', 'depreciation'],
    queryFn: () => reportsApi.depreciation(),
  })

  const exportDepreciationMutation = useMutation({
    mutationFn: () => reportsApi.exportDepreciation(),
    onSuccess: (blob) => {
      downloadBlob(blob, 'depreciation_report.csv')
      toast.success('折旧报表导出成功')
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const exportAssetsMutation = useMutation({
    mutationFn: () => assetsApi.exportAssets(),
    onSuccess: (blob) => {
      downloadBlob(blob, 'assets_export.csv')
      toast.success('资产导出成功')
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const s = summaryData

  const depreciationColumns: Column<reportsApi.DepreciationItem>[] = [
    { key: 'asset_name', label: '名称' },
    { key: 'asset_tag', label: '编号' },
    {
      key: 'purchase_price',
      label: '原值',
      render: (row) => row.purchase_price != null ? `¥${row.purchase_price.toLocaleString()}` : '—',
    },
    {
      key: 'monthly_depreciation',
      label: '月折旧',
      render: (row) => row.monthly_depreciation != null ? `¥${row.monthly_depreciation.toLocaleString()}` : '—',
    },
    { key: 'depreciated_months', label: '已提月数' },
    {
      key: 'net_book_value',
      label: '净值',
      render: (row) => row.net_book_value != null ? `¥${row.net_book_value.toLocaleString()}` : '—',
    },
  ]

  const formatCurrency = (val: unknown) => {
    const n = Number(val)
    return Number.isFinite(n) ? `¥${n.toLocaleString()}` : '—'
  }

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.24px', margin: '0 0 4px' }}>
            报表
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>资产统计与折旧报表</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          {isAdmin && (
            <>
              <Button
                variant="secondary"
                loading={exportAssetsMutation.isPending}
                onClick={() => exportAssetsMutation.mutate()}
              >
                导出资产 CSV
              </Button>
              <Button
                loading={exportDepreciationMutation.isPending}
                onClick={() => exportDepreciationMutation.mutate()}
              >
                导出折旧报表 CSV
              </Button>
            </>
          )}
        </div>
      </div>

      {/* KPI Cards */}
      {summaryLoading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
          <div style={{ width: 24, height: 24, border: '2px solid var(--border-default)', borderTopColor: 'var(--brand)', borderRadius: '50%', animation: 'spin 0.6s linear infinite' }} />
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 16, marginBottom: 32 }}>
          <KpiCard label="总资产数" value={s ? String(s.total_assets) : '—'} color="var(--brand)" />
          <KpiCard label="总原值" value={s ? formatCurrency(s.total_purchase_price) : '—'} color="#7170ff" />
          <KpiCard label="累计折旧" value={s ? formatCurrency(s.total_depreciation) : '—'} color="var(--warning)" />
          <KpiCard label="净值" value={s ? formatCurrency(s.net_book_value) : '—'} color="#16a34a" />
        </div>
      )}

      {/* Depreciation Table */}
      <div style={{ marginBottom: 8 }}>
        <h2 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-secondary)', margin: '0 0 16px', letterSpacing: '-0.15px' }}>
          折旧明细表
        </h2>
      </div>
      <DataTable
        columns={depreciationColumns}
        rows={depreciationData || []}
        loading={depreciationLoading}
        emptyState={{ title: '暂无折旧数据', description: '创建带有折旧信息的资产后即可查看' }}
      />

      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
