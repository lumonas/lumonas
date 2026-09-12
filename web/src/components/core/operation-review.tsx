import { Check } from 'lucide-react'
import { AlertBanner } from '@/components/core/alert-banner'
import { DiffViewer } from '@/components/core/diff-viewer'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface ReviewWarning {
  label: string
  description?: string
}

export function OperationReview({
  open,
  onOpenChange,
  title,
  description,
  steps,
  warnings,
  diff,
  children,
  confirmLabel = 'Apply',
  loading = false,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  steps?: string[]
  warnings?: ReviewWarning[]
  diff?: { before: string; after: string }
  children?: React.ReactNode
  confirmLabel?: string
  loading?: boolean
  onConfirm: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Review changes</DialogTitle>
          <DialogDescription>{description ?? title}</DialogDescription>
        </DialogHeader>

        {steps && steps.length > 0 && (
          <ul className="flex flex-col gap-1.5">
            {steps.map((step) => (
              <li key={step} className="flex items-start gap-2 text-sm">
                <Check className="mt-0.5 size-4 shrink-0 text-success" />
                <span>{step}</span>
              </li>
            ))}
          </ul>
        )}

        {warnings && warnings.length > 0 && (
          <div className="flex flex-col gap-2">
            {warnings.map((warning) => (
              <AlertBanner key={warning.label} tone="warning" title={warning.label}>
                {warning.description}
              </AlertBanner>
            ))}
          </div>
        )}

        {diff && <DiffViewer before={diff.before} after={diff.after} />}

        {children}

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={loading}>
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
