import { ReactNode } from 'react'

interface FormFieldProps {
  label: string
  error?: string
  required?: boolean
  children: ReactNode
}

export default function FormField({ label, error, required, children }: FormFieldProps) {
  return (
    <div style={{ marginBottom: 14 }}>
      <label
        style={{
          display: 'block',
          fontSize: 12,
          fontWeight: 500,
          color: error ? 'var(--danger)' : 'var(--text-secondary)',
          marginBottom: 5,
        }}
      >
        {label}
        {required && (
          <span style={{ color: 'var(--danger)', marginLeft: 2 }}>*</span>
        )}
      </label>
      {children}
      {error && (
        <p style={{ fontSize: 11, color: 'var(--danger)', margin: '4px 0 0' }}>{error}</p>
      )}
    </div>
  )
}
