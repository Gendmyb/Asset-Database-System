const statusMap: Record<string, { bg: string; color: string; label: string }> = {
  available: { bg: 'rgba(39,166,68,0.1)', color: '#4ade80', label: '可用' },
  assigned: { bg: 'rgba(94,106,210,0.1)', color: '#818cf8', label: '已领用' },
  borrowed: { bg: 'rgba(168,85,247,0.1)', color: '#c084fc', label: '已出借' },
  maintenance: { bg: 'rgba(245,158,11,0.1)', color: '#fbbf24', label: '维护中' },
  retired: { bg: 'rgba(255,255,255,0.05)', color: '#8a8f98', label: '已退役' },
  broken: { bg: 'rgba(239,68,68,0.1)', color: '#f87171', label: '已损坏' },
}

const assignmentMap: Record<string, { bg: string; color: string; label: string }> = {
  permanent: { bg: 'rgba(94,106,210,0.1)', color: '#818cf8', label: '领用' },
  temporary: { bg: 'rgba(168,85,247,0.1)', color: '#c084fc', label: '借用' },
}

const maintenanceStatusMap: Record<string, { bg: string; color: string; label: string }> = {
  open: { bg: 'rgba(59,130,246,0.1)', color: '#60a5fa', label: '待处理' },
  in_progress: { bg: 'rgba(245,158,11,0.1)', color: '#fbbf24', label: '处理中' },
  completed: { bg: 'rgba(39,166,68,0.1)', color: '#4ade80', label: '已完成' },
  canceled: { bg: 'rgba(255,255,255,0.05)', color: '#8a8f98', label: '已取消' },
}

const maintenanceCategoryMap: Record<string, { bg: string; color: string; label: string }> = {
  repair: { bg: 'rgba(239,68,68,0.1)', color: '#f87171', label: '维修' },
  upkeep: { bg: 'rgba(59,130,246,0.1)', color: '#60a5fa', label: '保养' },
}

interface BadgeProps {
  status?: string
  type?: 'status' | 'assignment' | 'maintenance_status' | 'maintenance_category'
}

export default function Badge({ status, type = 'status' }: BadgeProps) {
  if (type === 'assignment') {
    const a = assignmentMap[status || ''] || {
      bg: 'rgba(255,255,255,0.05)',
      color: 'var(--text-tertiary)',
      label: status || '—',
    }
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            background: a.color,
          }}
        />
        <span style={{ color: a.color, fontWeight: 500 }}>{a.label}</span>
      </span>
    )
  }

  if (type === 'maintenance_status') {
    const s = maintenanceStatusMap[status || ''] || {
      bg: 'rgba(255,255,255,0.05)',
      color: 'var(--text-tertiary)',
      label: status || '—',
    }
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
        <span
          style={{
            width: 6,
            height: 6,
            borderRadius: '50%',
            background: s.color,
          }}
        />
        <span style={{ color: s.color, fontWeight: 500 }}>{s.label}</span>
      </span>
    )
  }

  if (type === 'maintenance_category') {
    const c = maintenanceCategoryMap[status || ''] || {
      bg: 'rgba(255,255,255,0.05)',
      color: 'var(--text-tertiary)',
      label: status || '—',
    }
    return (
      <span
        style={{
          display: 'inline-block',
          padding: '2px 8px',
          borderRadius: 4,
          background: c.bg,
          color: c.color,
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: '0.3px',
        }}
      >
        {c.label}
      </span>
    )
  }

  const s = statusMap[status || ''] || {
    bg: 'rgba(255,255,255,0.05)',
    color: 'var(--text-tertiary)',
    label: status || '—',
  }

  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          background: s.color,
        }}
      />
      <span style={{ color: s.color }}>{s.label}</span>
    </span>
  )
}
