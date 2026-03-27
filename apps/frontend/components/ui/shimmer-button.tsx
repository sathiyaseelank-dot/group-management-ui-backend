import { cn } from '@/lib/utils'

interface ShimmerButtonProps extends React.ComponentProps<'button'> {
  shimmerColor?: string
  shimmerSize?: string
  shimmerDuration?: string
}

export function ShimmerButton({
  children,
  className,
  shimmerColor = 'rgba(255,255,255,0.12)',
  shimmerSize = '60%',
  shimmerDuration = '2.5s',
  ...props
}: ShimmerButtonProps) {
  return (
    <button
      className={cn(
        'relative overflow-hidden rounded-md bg-primary text-primary-foreground',
        'inline-flex items-center justify-center gap-2',
        'transition-all hover:shadow-lg hover:shadow-primary/20',
        'disabled:pointer-events-none disabled:opacity-50',
        className,
      )}
      {...props}
    >
      {/* Shimmer sweep */}
      <div
        className="pointer-events-none absolute inset-0"
        style={{
          background: `linear-gradient(120deg, transparent 25%, ${shimmerColor} 50%, transparent 75%)`,
          backgroundSize: `${shimmerSize} 100%`,
          animation: `shimmer-sweep ${shimmerDuration} ease-in-out infinite`,
        }}
      />
      <span className="relative z-10 flex items-center gap-2">{children}</span>
    </button>
  )
}
