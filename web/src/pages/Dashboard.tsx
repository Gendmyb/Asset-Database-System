import { useQuery } from '@tanstack/react-query'
import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import api from '../api/client'

const STATUS_COLORS: Record<string, string> = {
  available: '#4ade80',
  assigned: '#818cf8',
  maintenance: '#fbbf24',
  broken: '#f87171',
  borrowed: '#c084fc',
  retired: '#8a8f98',
}

const STATUS_LABELS: Record<string, string> = {
  available: '可用',
  assigned: '已领用',
  maintenance: '维护中',
  broken: '已损坏',
  borrowed: '已出借',
  retired: '已退役',
}

const LIFECYCLE_LABELS: Record<string, string> = {
  deployment: '部署中',
  utilization: '使用中',
  maintenance: '维护中',
  retirement: '已退役',
}

const TYPE_COLORS = ['#5e6ad2', '#7170ff', '#4ade80', '#fbbf24', '#f87171', '#c084fc', '#38bdf8', '#a78bfa']

export default function Dashboard() {
  const { data: stats, isLoading } = useQuery({
    queryKey: ['dashboard', 'overview'],
    queryFn: () => api.get('/dashboard/overview').then((r) => r.data?.data || r.data),
  })

  const s = stats as Record<string, unknown> | null

  // Build pie data from by_status
  const pieData = s?.by_status
    ? Object.entries(s.by_status as Record<string, number>).map(([key, value]) => ({
        name: STATUS_LABELS[key] || key,
        value,
        color: STATUS_COLORS[key] || 'var(--text-quaternary)',
      }))
    : []

  // Build bar data from by_type or lifecycle
  const barData = s?.by_type
    ? Object.entries(s.by_type as Record<string, number>).map(([key, value]) => ({
        name: key,
        count: value,
      }))
    : s?.by_lifecycle
      ? Object.entries(s.by_lifecycle as Record<string, number>).map(([key, value]) => ({
          name: LIFECYCLE_LABELS[key] || key,
          count: value,
        }))
      : []

  // KPI data
  const totalAssets = Number(s?.total_assets || 0)
  const totalPurchasePrice = Number(s?.total_purchase_price || 0)
  const totalDepreciation = Number(s?.total_depreciation || 0)
  const netValue = totalPurchasePrice - totalDepreciation
  const recentAdditions = Number(s?.recent_additions || 0)
  const availableCount = s?.by_status
    ? (s.by_status as Record<string, number>).available || 0
    : 0
  const availableRate = totalAssets > 0
    ? Math.round((availableCount / totalAssets) * 100)
    : 0
  const maintenanceCount = s?.by_status
    ? (s.by_status as Record<string, number>).maintenance || 0
    : 0

  return (
    <div style={{ padding: 32, maxWidth: 1200 }}>
      <div style={{ marginBottom: 32 }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.24px', marginBottom: 4 }}>
          仪表盘
        </h1>
        <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>IT 资产概览</p>
      </div>

      {isLoading ? (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
          <div style={{ width: 24, height: 24, border: '2px solid var(--border-default)', borderTopColor: 'var(--brand)', borderRadius: '50%', animation: 'spin 0.6s linear infinite' }} />
        </div>
      ) : (
        <>
          {/* KPI Cards */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 16, marginBottom: 16 }}>
            <KpiCard label="资产总数" value={String(totalAssets)} color="var(--brand)" />
            <KpiCard label="资产原值" value={`¥${totalPurchasePrice.toLocaleString()}`} color="#7170ff" />
            <KpiCard label="净值" value={`¥${netValue.toLocaleString()}`} color="#4ade80" />
            <KpiCard label="维护中" value={String(maintenanceCount)} color="var(--warning)" />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 16, marginBottom: 32 }}>
            <KpiCard label="可用率" value={`${availableRate}%`} color="#7170ff" />
            <KpiCard label="累计折旧" value={`¥${totalDepreciation.toLocaleString()}`} color="var(--warning)" />
            <KpiCard label="近30天新增" value={`${recentAdditions} 件`} color="#38bdf8" />
          </div>

          {/* Charts Row 1: Pie + Bar */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 32 }}>
            {/* Status Distribution Pie Chart */}
            <Panel title="状态分布">
              {pieData.length > 0 ? (
                <ResponsiveContainer width="100%" height={260}>
                  <PieChart>
                    <Pie
                      data={pieData}
                      cx="50%"
                      cy="50%"
                      innerRadius={55}
                      outerRadius={90}
                      paddingAngle={3}
                      dataKey="value"
                      label={({ name, value }) => `${name} ${value}`}
                      labelLine={{ stroke: 'var(--text-quaternary)', strokeWidth: 1 }}
                    >
                      {pieData.map((entry, i) => (
                        <Cell key={i} fill={entry.color} />
                      ))}
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        background: 'var(--bg-surface)',
                        border: '1px solid var(--border-default)',
                        borderRadius: 8,
                        color: 'var(--text-primary)',
                        fontSize: 13,
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
              ) : (
                <EmptyState />
              )}
            </Panel>

            {/* Bar Chart: Type or Lifecycle distribution */}
            <Panel title={s?.by_type ? '资产类型分布' : '生命周期分布'}>
              {barData.length > 0 ? (
                <ResponsiveContainer width="100%" height={260}>
                  <BarChart data={barData} layout="vertical" margin={{ left: 10, right: 10 }}>
                    <XAxis type="number" tick={{ fontSize: 11, fill: 'var(--text-quaternary)' }} axisLine={false} tickLine={false} />
                    <YAxis type="category" dataKey="name" tick={{ fontSize: 12, fill: 'var(--text-secondary)' }} axisLine={false} tickLine={false} width={80} />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--bg-surface)',
                        border: '1px solid var(--border-default)',
                        borderRadius: 8,
                        color: 'var(--text-primary)',
                        fontSize: 13,
                      }}
                    />
                    <Bar dataKey="count" radius={[0, 4, 4, 0]}>
                      {barData.map((_, i) => (
                        <Cell key={i} fill={TYPE_COLORS[i % TYPE_COLORS.length]} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <EmptyState />
              )}
            </Panel>
          </div>

          {/* Lifecycle Grid (keep existing style) */}
          <Panel title="生命周期分布明细">
            {s?.by_lifecycle ? (
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px,1fr))', gap: 8 }}>
                {Object.entries(s.by_lifecycle as Record<string, number>).map(([k, v]) => (
                  <div key={k} style={{ background: 'rgba(0,0,0,0.02)', borderRadius: 8, padding: 14, border: '1px solid var(--border-subtle)' }}>
                    <div style={{ fontSize: 24, fontWeight: 600, color: 'var(--text-primary)' }}>{v}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 2 }}>{LIFECYCLE_LABELS[k] || k}</div>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState />
            )}
          </Panel>
        </>
      )}
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}

function KpiCard({ label, value, color }: { label: string; value: string; color: string }) {
  return (
    <div style={{
      background: 'var(--bg-surface)', borderRadius: 10, padding: '20px 24px',
      border: '1px solid var(--border-subtle)', borderLeft: `3px solid ${color}`,
    }}>
      <div style={{ fontSize: 12, color: 'var(--text-tertiary)', fontWeight: 500, marginBottom: 8 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.5px' }}>{value}</div>
    </div>
  )
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: 'var(--bg-surface)', borderRadius: 10, padding: 24,
      border: '1px solid var(--border-subtle)',
    }}>
      <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-secondary)', marginBottom: 16, letterSpacing: '-0.15px' }}>{title}</h3>
      {children}
    </div>
  )
}

function EmptyState() {
  return <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-quaternary)', fontSize: 13 }}>暂无数据</div>
}
