import * as React from 'react'
import { cn } from '@/lib/utils'

function Kbd({ className, ...props }: React.ComponentProps<'kbd'>) {
  return (
    <kbd
      className={cn(
        'pointer-events-none inline-flex h-5 select-none items-center gap-1 rounded border bg-muted px-1.5 font-sans text-[11px] font-medium text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}

export { Kbd }
