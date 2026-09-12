import { useId } from 'react'
import { cn } from '@/lib/utils'

export function Logo({ className }: { className?: string }) {
  const gradientId = useId()
  return (
    <svg viewBox="0 0 32 32" className={cn('size-7', className)} aria-hidden>
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" style={{ stopColor: 'var(--primary)' }} />
          <stop offset="100%" style={{ stopColor: 'var(--info)' }} />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="30" height="30" rx="8" fill={`url(#${gradientId})`} />
      <rect
        x="8"
        y="10"
        width="16"
        height="3"
        rx="1.5"
        fill="var(--primary-foreground)"
        opacity="0.9"
      />
      <rect
        x="8"
        y="16"
        width="10"
        height="3"
        rx="1.5"
        fill="var(--primary-foreground)"
        opacity="0.55"
      />
      <circle cx="23" cy="17.5" r="2.2" fill="var(--background)" />
    </svg>
  )
}
