import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium w-fit whitespace-nowrap [&>svg]:size-3',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary/10 text-primary',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'text-foreground',
        destructive: 'border-transparent bg-destructive/10 text-destructive',
        success: 'border-success/25 bg-success/10 text-success',
        attention: 'border-attention/25 bg-attention/10 text-attention',
        warning: 'border-warning/25 bg-warning/10 text-warning',
        critical: 'border-critical/25 bg-critical/10 text-critical',
        info: 'border-info/25 bg-info/10 text-info',
        offline: 'border-offline/25 bg-offline/10 text-offline',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
)

function Badge({
  className,
  variant,
  ...props
}: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}

export { Badge, badgeVariants }
