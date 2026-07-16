// 仪表盘页面
import { useEffect, useState } from 'react'
import api from '../api/client'

export default function Dashboard() {
  const [stats, setStats] = useState<Record<string, unknown> | null>(null)

  useEffect(() => {
    api.get('/dashboard/overview').then(({ data }) => setStats(data.data))
  }, [])

  if (!stats) return <div className="p-8 text-gray-500">加载中...</div>

  const s = stats as Record<string, unknown>

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">仪表盘</h1>

      {/* KPI 卡片 */}
      <div className="grid grid-cols-4 gap-4 mb-8">
        <StatCard label="总资产数" value={String(s.total_assets || 0)} color="blue" />
        <StatCard label="在线 Agent" value="—" color="green" />
        <StatCard label="今日变更" value="—" color="yellow" />
        <StatCard label="待审批" value="—" color="red" />
      </div>

      {/* 状态分布 */}
      <div className="bg-white rounded-lg shadow p-6 mb-6">
        <h2 className="font-bold mb-4">资产状态分布</h2>
        <div className="grid grid-cols-2 gap-4">
          {Object.entries((s.by_status as Record<string, number>) || {}).map(([k, v]) => (
            <div key={k} className="flex justify-between">
              <span className="text-gray-600">{k}</span>
              <span className="font-bold">{v}</span>
            </div>
          ))}
        </div>
      </div>

      {/* 生命周期分布 */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="font-bold mb-4">生命周期分布</h2>
        <div className="grid grid-cols-2 gap-4">
          {Object.entries((s.by_lifecycle as Record<string, number>) || {}).map(([k, v]) => (
            <div key={k} className="flex justify-between">
              <span className="text-gray-600">{k}</span>
              <span className="font-bold">{v}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function StatCard({ label, value, color }: { label: string; value: string; color: string }) {
  const borders: Record<string, string> = {
    blue: 'border-l-blue-500',
    green: 'border-l-green-500',
    yellow: 'border-l-yellow-500',
    red: 'border-l-red-500',
  }
  return (
    <div className={`bg-white rounded-lg shadow p-4 border-l-4 ${borders[color]}`}>
      <div className="text-sm text-gray-500">{label}</div>
      <div className="text-2xl font-bold">{value}</div>
    </div>
  )
}
