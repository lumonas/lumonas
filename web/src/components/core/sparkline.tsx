import { useId } from 'react'
import { cn } from '@/lib/utils'

export function Sparkline({
  data,
  className,
  strokeWidth = 1.5,
}: {
  data: number[]
  className?: string
  strokeWidth?: number
}) {
  const gradientId = useId()
  const width = 100
  const height = 28
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = Math.max(max - min, 1)
  const step = data.length > 1 ? width / (data.length - 1) : width
  const points = data.map((value, i) => {
    const x = i * step
    const y = height - ((value - min) / range) * (height - 2) - 1
    return `${x.toFixed(2)},${y.toFixed(2)}`
  })
  const line = `M ${points.join(' L ')}`
  const area = `${line} L ${width},${height} L 0,${height} Z`

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={cn('h-7 w-full', className)}
      aria-hidden
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.25" />
          <stop offset="100%" stopColor="var(--primary)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${gradientId})`} />
      <path
        d={line}
        fill="none"
        stroke="var(--primary)"
        strokeWidth={strokeWidth}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}
