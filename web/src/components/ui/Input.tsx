import React from 'react'

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string
  error?: string
}

export default function Input({ label, error, style, ...rest }: InputProps) {
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
      <input
        {...rest}
        style={{
          width: '100%',
          padding: '7px 10px',
          borderRadius: 5,
          border: `1px solid ${error ? 'var(--danger)' : 'var(--border-default)'}`,
          background: 'rgba(0,0,0,0.02)',
          color: 'var(--text-primary)',
          fontSize: 13,
          outline: 'none',
          fontFamily: 'inherit',
          transition: 'border-color .15s',
          ...style,
        }}
        onFocus={(e) => {
          if (!error) {
            e.currentTarget.style.borderColor = 'var(--brand)'
            e.currentTarget.style.borderWidth = '2px'
            e.currentTarget.style.padding = '6px 9px'
          }
        }}
        onBlur={(e) => {
          e.currentTarget.style.borderColor = error ? 'var(--danger)' : 'var(--border-default)'
          e.currentTarget.style.borderWidth = '1px'
          e.currentTarget.style.padding = '7px 10px'
        }}
      />
      {error && (
        <p style={{ fontSize: 11, color: 'var(--danger)', margin: '4px 0 0' }}>{error}</p>
      )}
    </div>
  )
}
