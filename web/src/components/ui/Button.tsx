import React from 'react'
import Spinner from './Spinner'

interface ButtonProps {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  loading?: boolean
  disabled?: boolean
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void
  children: React.ReactNode
  type?: 'button' | 'submit' | 'reset'
  style?: React.CSSProperties
}

const baseStyle: React.CSSProperties = {
  padding: '8px 16px',
  borderRadius: 6,
  border: 'none',
  cursor: 'pointer',
  fontSize: 13,
  fontWeight: 500,
  fontFamily: 'inherit',
  transition: 'background .15s, opacity .15s',
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 6,
}

const variantStyles: Record<string, React.CSSProperties> = {
  primary: {
    background: 'var(--brand)',
    color: 'white',
  },
  secondary: {
    background: 'var(--bg-surface)',
    color: 'var(--text-secondary)',
    border: '1px solid var(--border-default)',
  },
  danger: {
    background: 'var(--danger)',
    color: 'white',
  },
  ghost: {
    background: 'transparent',
    color: 'var(--text-tertiary)',
  },
}

export default function Button({
  variant = 'primary',
  loading = false,
  disabled,
  onClick,
  children,
  type = 'button',
  style,
}: ButtonProps) {
  const isDisabled = disabled || loading

  return (
    <button
      type={type}
      disabled={isDisabled}
      onClick={onClick}
      style={{
        ...baseStyle,
        ...variantStyles[variant],
        ...(isDisabled ? { opacity: 0.5, cursor: 'default' } : {}),
        ...style,
      }}
    >
      {loading && <Spinner size={14} />}
      {children}
    </button>
  )
}
