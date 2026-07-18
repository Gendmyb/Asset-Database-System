import React from 'react'

interface SelectOption {
  value: string
  label: string
}

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string
  error?: string
  options: SelectOption[]
}

export default function Select({ label, error, options, style, ...rest }: SelectProps) {
  return (
    <div style={{ marginBottom: 14 }}>
      {label && (
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
        </label>
      )}
      <select
        {...rest}
        style={{
          width: '100%',
          padding: '7px 10px',
          borderRadius: 5,
          border: `1px solid ${error ? 'var(--danger)' : 'var(--border-default)'}`,
          background: 'rgba(255,255,255,0.02)',
          color: 'var(--text-primary)',
          fontSize: 13,
          outline: 'none',
          fontFamily: 'inherit',
          cursor: 'pointer',
          ...style,
        }}
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      {error && (
        <p style={{ fontSize: 11, color: 'var(--danger)', margin: '4px 0 0' }}>{error}</p>
      )}
    </div>
  )
}
