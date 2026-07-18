import { Link } from 'react-router-dom'

export default function MaintenancePage() {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '60vh',
        padding: 32,
        textAlign: 'center',
      }}
    >
      <div
        style={{
          fontSize: 48,
          fontWeight: 600,
          color: 'var(--text-quaternary)',
          marginBottom: 16,
        }}
      >
        🔧
      </div>
      <h1 style={{ fontSize: 20, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 8px' }}>
        维修保养
      </h1>
      <p style={{ fontSize: 14, color: 'var(--text-tertiary)', marginBottom: 24 }}>
        维修保养功能开发中，敬请期待
      </p>
      <Link
        to="/assets"
        style={{
          padding: '10px 20px',
          borderRadius: 6,
          background: 'var(--brand)',
          color: 'white',
          textDecoration: 'none',
          fontSize: 14,
          fontWeight: 500,
        }}
      >
        返回资产列表
      </Link>
    </div>
  )
}
