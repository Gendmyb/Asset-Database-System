import { useEffect, useCallback, ReactNode } from 'react'

interface DrawerProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  width?: string
}

export default function Drawer({ open, onClose, title, children, width = '400px' }: DrawerProps) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    },
    [onClose]
  )

  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown)
      return () => document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, handleKeyDown])

  if (!open) return null

  return (
    <>
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          zIndex: 49,
          background: 'rgba(0,0,0,0.5)',
          backdropFilter: 'blur(4px)',
        }}
      />
      <div
        className="drawer-panel"
        style={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width,
          zIndex: 50,
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border-default)',
          boxShadow: '-12px 0 40px rgba(0,0,0,0.6)',
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <div
          style={{
            position: 'sticky',
            top: 0,
            background: 'var(--bg-panel)',
            borderBottom: '1px solid var(--border-subtle)',
            padding: '16px 20px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            zIndex: 1,
            flexShrink: 0,
          }}
        >
          <h3 style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            {title}
          </h3>
          <button
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-quaternary)',
              cursor: 'pointer',
              fontSize: 16,
              padding: 0,
            }}
          >
            &#x2715;
          </button>
        </div>
        <div style={{ padding: 20, flex: 1 }}>{children}</div>
      </div>
    </>
  )
}
