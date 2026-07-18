const statusMap: Record<string, { bg: string; color: string; label: string }> = {
  available: { bg: 'rgba(39,166,68,0.1)', color: '#4ade80', label: '可用' },
  assigned: { bg: 'rgba(94,106,210,0.1)', color: '#818cf8', label: '已领用' },
  borrowed: { bg: 'rgba(168,85,247,0.1)', color: '#c084fc', label: '已出借' },
  maintenance: { bg: 'rgba(245,158,11,0.1)', color: '#fbbf24', label: '维护中' },
  retired: { bg: 'rgba(255,255,255,0.05)', color: '#8a8f98', label: '已退役' },
  broken: { bg: 'rgba(239,68,68,0.1)', color: '#f87171', label: '已损坏' },
}

interface BadgeProps {
  status: string
}

export default function Badge({ status }: BadgeProps) {
  const s = statusMap[status] || {
    bg: 'rgba(255,255,255,0.05)',
    color: 'var(--text-tertiary)',
    label: status,
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
