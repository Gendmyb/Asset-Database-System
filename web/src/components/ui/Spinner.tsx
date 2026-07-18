interface SpinnerProps {
  size?: number
}

export default function Spinner({ size = 20 }: SpinnerProps) {
  return (
    <div
      style={{
        width: size,
        height: size,
        border: '2px solid var(--border-default)',
        borderTopColor: 'var(--brand)',
        borderRadius: '50%',
        animation: 'spin 0.6s linear infinite',
      }}
    >
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
