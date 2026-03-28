import { cn } from '@/lib/utils'

interface BorderBeamProps {
  className?: string
  size?: number
  duration?: number
  delay?: number
  colorFrom?: string
  colorTo?: string
}

export function BorderBeam({
  className,
  size = 200,
  duration = 12,
  delay = 0,
  colorFrom = 'oklch(0.68 0.19 250)',
  colorTo = 'oklch(0.72 0.20 150)',
}: BorderBeamProps) {
  return (
    <div
      className={cn(
        'pointer-events-none absolute inset-0 rounded-[inherit]',
        className,
      )}
      style={{
        WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
        WebkitMaskComposite: 'xor',
        maskComposite: 'exclude',
        padding: '1px',
      }}
    >
      <div
        className="absolute inset-[-100%] animate-border-beam"
        style={{
          background: `conic-gradient(from 0deg, transparent 0%, transparent 50%, ${colorFrom} 60%, ${colorTo} 70%, transparent 80%, transparent 100%)`,
          animationDuration: `${duration}s`,
          animationDelay: `${delay}s`,
          width: `${size}%`,
          height: `${size}%`,
        }}
      />
    </div>
  )
}
