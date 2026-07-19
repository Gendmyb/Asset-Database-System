import Input from '../ui/Input'
import Select from '../ui/Select'

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'available', label: '可用' },
  { value: 'assigned', label: '已领用' },
  { value: 'maintenance', label: '维护中' },
]

const LIFECYCLE_OPTIONS = [
  { value: '', label: '全部生命周期' },
  { value: 'deployment', label: '部署中' },
  { value: 'utilization', label: '使用中' },
  { value: 'maintenance', label: '维护中' },
  { value: 'retirement', label: '已退役' },
]

const MANUFACTURER_OPTIONS = [
  { value: '', label: '全部制造商' },
  { value: 'Apple', label: 'Apple' },
  { value: 'Dell', label: 'Dell' },
  { value: 'Lenovo', label: 'Lenovo' },
  { value: 'Cisco', label: 'Cisco' },
]

interface AssetFiltersProps {
  search: string
  onSearchChange: (v: string) => void
  status: string
  onStatusChange: (v: string) => void
  lifecycle: string
  onLifecycleChange: (v: string) => void
  manufacturer: string
  onManufacturerChange: (v: string) => void
}

export default function AssetFilters({
  search,
  onSearchChange,
  status,
  onStatusChange,
  lifecycle,
  onLifecycleChange,
  manufacturer,
  onManufacturerChange,
}: AssetFiltersProps) {
  return (
    <div style={{ display: 'flex', gap: 10, marginBottom: 16, flexWrap: 'wrap' }}>
      <div style={{ flex: 1, minWidth: 220, maxWidth: 360 }}>
        <Input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="搜索名称、标签、制造商..."
          style={{
            paddingLeft: 34,
            background: 'rgba(0,0,0,0.02)',
            position: 'relative',
          }}
        />
        <svg
          style={{
            position: 'absolute',
            left: 10,
            top: '50%',
            transform: 'translateY(-50%)',
            width: 14,
            height: 14,
            color: 'var(--text-quaternary)',
          }}
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
        >
          <circle cx="11" cy="11" r="8" />
          <path d="m21 21-4.35-4.35" />
        </svg>
      </div>
      <Select
        options={STATUS_OPTIONS}
        value={status}
        onChange={(e) => onStatusChange(e.target.value)}
        style={{
          padding: '7px 10px',
          borderRadius: 6,
          border: '1px solid var(--border-default)',
          background: `rgba(0,0,0,${status ? '0.04' : '0.02'})`,
          color: status ? 'var(--text-primary)' : 'var(--text-tertiary)',
          fontSize: 13,
          fontFamily: 'inherit',
          cursor: 'pointer',
          minWidth: 110,
          outline: 'none',
        }}
      />
      <Select
        options={LIFECYCLE_OPTIONS}
        value={lifecycle}
        onChange={(e) => onLifecycleChange(e.target.value)}
        style={{
          padding: '7px 10px',
          borderRadius: 6,
          border: '1px solid var(--border-default)',
          background: `rgba(0,0,0,${lifecycle ? '0.04' : '0.02'})`,
          color: lifecycle ? 'var(--text-primary)' : 'var(--text-tertiary)',
          fontSize: 13,
          fontFamily: 'inherit',
          cursor: 'pointer',
          minWidth: 120,
          outline: 'none',
        }}
      />
      <Select
        options={MANUFACTURER_OPTIONS}
        value={manufacturer}
        onChange={(e) => onManufacturerChange(e.target.value)}
        style={{
          padding: '7px 10px',
          borderRadius: 6,
          border: '1px solid var(--border-default)',
          background: `rgba(0,0,0,${manufacturer ? '0.04' : '0.02'})`,
          color: manufacturer ? 'var(--text-primary)' : 'var(--text-tertiary)',
          fontSize: 13,
          fontFamily: 'inherit',
          cursor: 'pointer',
          minWidth: 110,
          outline: 'none',
        }}
      />
    </div>
  )
}
