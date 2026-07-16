// 资产管理页面 — 表格 + 创建/编辑模态框
import { useEffect, useState } from 'react'
import api from '../api/client'
import type { Asset, PaginatedResponse } from '../types'

export default function Assets() {
  const [assets, setAssets] = useState<Asset[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [cursor, setCursor] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)

  const fetchAssets = async (reset = false) => {
    setLoading(true)
    const { data } = await api.get<PaginatedResponse<Asset>>('/assets', {
      params: { search: search || undefined, cursor: reset ? undefined : cursor, limit: 20 },
    })
    setAssets((prev) => reset ? data.data : [...prev, ...data.data])
    setCursor(data.pagination.next_cursor)
    setLoading(false)
  }

  useEffect(() => { fetchAssets(true) }, [])

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">资产管理</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-blue-600 text-white px-4 py-2 rounded hover:bg-blue-700"
        >
          + 创建资产
        </button>
      </div>

      {/* 搜索 */}
      <input
        type="text"
        placeholder="搜索资产名称或标签..."
        value={search}
        onChange={(e) => { setSearch(e.target.value); fetchAssets(true) }}
        className="w-full border rounded px-3 py-2 mb-4"
      />

      {/* 资产表格 */}
      <div className="bg-white rounded-lg shadow overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">标签</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">名称</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">制造商</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">型号</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">状态</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">生命周期</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {assets.map((a) => (
              <tr key={a.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 text-sm font-mono">{a.asset_tag}</td>
                <td className="px-4 py-3 text-sm">{a.name}</td>
                <td className="px-4 py-3 text-sm text-gray-600">{a.manufacturer || '-'}</td>
                <td className="px-4 py-3 text-sm text-gray-600">{a.model || '-'}</td>
                <td className="px-4 py-3 text-sm">
                  <StatusBadge status={a.status} />
                </td>
                <td className="px-4 py-3 text-sm text-gray-600">{a.lifecycle_state}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {loading && <div className="p-4 text-center text-gray-500">加载中...</div>}
        {cursor && !loading && (
          <button onClick={() => fetchAssets()} className="w-full p-3 text-blue-600 hover:bg-gray-50">
            加载更多
          </button>
        )}
      </div>

      {/* 创建模态框 (简化) */}
      {showCreate && <CreateModal onClose={() => setShowCreate(false)} onCreated={() => { setShowCreate(false); fetchAssets(true) }} />}
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    available: 'bg-green-100 text-green-800',
    assigned: 'bg-blue-100 text-blue-800',
    maintenance: 'bg-yellow-100 text-yellow-800',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-medium ${colors[status] || 'bg-gray-100'}`}>
      {status}
    </span>
  )
}

function CreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [form, setForm] = useState({ asset_tag: '', name: '', type_id: 'type-001', manufacturer: '', model: '' })
  const [error, setError] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await api.post('/assets', form)
      onCreated()
    } catch {
      setError('创建失败')
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg p-6 w-96">
        <h2 className="text-lg font-bold mb-4">创建资产</h2>
        <form onSubmit={handleSubmit}>
          {(['asset_tag', 'name', 'manufacturer', 'model'] as const).map((f) => (
            <div key={f} className="mb-3">
              <label className="block text-sm font-medium mb-1">{f}</label>
              <input
                value={form[f]}
                onChange={(e) => setForm({ ...form, [f]: e.target.value })}
                className="w-full border rounded px-3 py-2 text-sm"
                required={f === 'asset_tag' || f === 'name'}
              />
            </div>
          ))}
          {error && <div className="text-red-500 text-sm mb-3">{error}</div>}
          <div className="flex gap-2">
            <button type="button" onClick={onClose} className="flex-1 border rounded py-2">取消</button>
            <button type="submit" className="flex-1 bg-blue-600 text-white rounded py-2">创建</button>
          </div>
        </form>
      </div>
    </div>
  )
}
