import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import AssetTable from '../components/assets/AssetTable'
import AssetDetailPanel from '../components/assets/AssetDetailPanel'
import CreateAssetModal from '../components/assets/CreateAssetModal'
import ImportWizard from '../components/assets/ImportWizard'
import AssignDialog from '../components/assets/AssignDialog'
import Button from '../components/ui/Button'
import * as assetsApi from '../api/assets'
import * as usersApi from '../api/users'
import * as lookupApi from '../api/lookup'
import { getApiError } from '../lib/errors'

export default function Assets() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [lifecycleFilter, setLifecycleFilter] = useState('')
  const [manufacturerFilter, setManufacturerFilter] = useState('')
  const [selected, setSelected] = useState<assetsApi.Asset | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showImport, setShowImport] = useState(false)
  const [showAssign, setShowAssign] = useState(false)
  const [assignMode, setAssignMode] = useState<'assign' | 'borrow'>('assign')
  const [assignAsset, setAssignAsset] = useState<assetsApi.Asset | null>(null)

  const { data: usersData } = useQuery({
    queryKey: ['users'],
    queryFn: () => usersApi.list(),
    staleTime: 60000,
  })
  const { data: locationsData } = useQuery({
    queryKey: ['locations'],
    queryFn: () => lookupApi.locations(),
    staleTime: 60000,
  })
  const { data: typesData } = useQuery({
    queryKey: ['assetTypes'],
    queryFn: () => lookupApi.assetTypes(),
    staleTime: 60000,
  })

  const userOptions = Array.isArray(usersData)
    ? usersData.map((u: any) => ({ value: u.id, label: u.username }))
    : []
  const locationOptions = Array.isArray(locationsData)
    ? locationsData.map((l: any) => ({ value: l.id, label: l.name }))
    : []
  const typeOptions = Array.isArray(typesData)
    ? typesData.map((t: any) => ({ value: t.id, label: t.name }))
    : [
        { value: '10000000-0000-4000-a000-000000000001', label: '通用资产' },
        { value: '10000000-0000-4000-a000-000000000002', label: '计算机' },
        { value: '10000000-0000-4000-a000-000000000003', label: '显示器' },
        { value: '10000000-0000-4000-a000-000000000004', label: '网络设备' },
      ]

  const handleRefresh = useCallback(async (id: string) => {
    try {
      const asset = await assetsApi.getById(id)
      setSelected(asset)
    } catch {
      toast.error('刷新资产失败')
    }
  }, [])

  const handleAssign = (asset: assetsApi.Asset) => {
    setAssignAsset(asset)
    setAssignMode('assign')
    setShowAssign(true)
  }

  const handleBorrow = (asset: assetsApi.Asset) => {
    setAssignAsset(asset)
    setAssignMode('borrow')
    setShowAssign(true)
  }

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
            Assets
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
            Manage and track all IT assets
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button variant="secondary" onClick={() => setShowImport(true)}>导入</Button>
          <Button onClick={() => setShowCreate(true)}>+ 新建资产</Button>
        </div>
      </div>

      {/* Asset Table */}
      <AssetTable
        search={search}
        onSearchChange={setSearch}
        status={statusFilter}
        onStatusChange={setStatusFilter}
        lifecycle={lifecycleFilter}
        onLifecycleChange={setLifecycleFilter}
        manufacturer={manufacturerFilter}
        onManufacturerChange={setManufacturerFilter}
        onSelect={setSelected}
      />

      {/* Detail Panel */}
      {selected && (
        <AssetDetailPanel
          asset={selected}
          onClose={() => setSelected(null)}
          onRefresh={handleRefresh}
          onAssign={handleAssign}
          onBorrow={handleBorrow}
          onRelease={async (id) => {
            try {
              await assetsApi.release(id)
              toast.success('归还成功')
              handleRefresh(id)
            } catch (err) {
              toast.error(getApiError(err))
            }
          }}
        />
      )}

      {/* Create Modal */}
      <CreateAssetModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        userOptions={userOptions}
        locationOptions={locationOptions}
        typeOptions={typeOptions}
      />

      {/* Import Wizard */}
      <ImportWizard open={showImport} onClose={() => setShowImport(false)} />

      {/* Assign/Borrow Dialog */}
      {assignAsset && (
        <AssignDialog
          asset={assignAsset}
          open={showAssign}
          onClose={() => {
            setShowAssign(false)
            setAssignAsset(null)
          }}
          mode={assignMode}
        />
      )}
    </div>
  )
}
