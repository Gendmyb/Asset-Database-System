import Button from './Button'

interface EmptyStateProps {
  icon?: string
  title: string
  description?: string
  action?: {
    label: string
    onClick: () => void
  }
}

export default function EmptyState({ icon = '📦', title, description, action }: EmptyStateProps) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '60px 16px',
        textAlign: 'center',
      }}
    >
      <div style={{ fontSize: 32, marginBottom: 8 }}>{icon}</div>
      <div style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>{title}</div>
      {description && (
        <div style={{ fontSize: 12, color: 'var(--text-quaternary)', marginTop: 4 }}>
          {description}
        </div>
      )}
      {action && (
        <div style={{ marginTop: 16 }}>
          <Button variant="primary" onClick={action.onClick}>
            {action.label}
          </Button>
        </div>
      )}
    </div>
  )
}
