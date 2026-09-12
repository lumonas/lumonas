import { useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

export interface DangerAction {
  id: string
  label: string
  description: string
  match: string
}

export function DangerZone({
  actions,
  identity,
  onAction,
  className,
}: {
  actions: DangerAction[]
  identity: React.ReactNode
  onAction?: (id: string) => void
  className?: string
}) {
  const [action, setAction] = useState<DangerAction | null>(null)
  const [confirmation, setConfirmation] = useState('')

  function close() {
    setAction(null)
    setConfirmation('')
  }

  return (
    <div
      className={cn(
        'rounded-xl border border-critical/30 bg-critical/[0.04] p-4',
        className,
      )}
    >
      <div className="flex items-center gap-2 text-sm font-medium text-critical">
        <TriangleAlert className="size-4" />
        Danger zone
      </div>
      <p className="mt-1 text-sm text-muted-foreground">
        Destructive operations are final. They are kept separate from normal actions on purpose.
      </p>
      <div className="mt-3 flex flex-col gap-2">
        {actions.map((item) => (
          <Button
            key={item.id}
            variant="destructiveOutline"
            size="sm"
            className="justify-start"
            onClick={() => {
              setAction(item)
              setConfirmation('')
            }}
          >
            {item.label}
          </Button>
        ))}
      </div>

      <Dialog open={action != null} onOpenChange={(open) => !open && close()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-destructive">{action?.label}</DialogTitle>
            <DialogDescription>{action?.description}</DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border bg-background p-3">{identity}</div>
          <div className="grid gap-2">
            <Label htmlFor="danger-confirm">
              Type <span className="font-mono text-foreground">{action?.match}</span> to confirm
            </Label>
            <Input
              id="danger-confirm"
              autoComplete="off"
              spellCheck={false}
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              placeholder={action?.match}
              className="font-mono"
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={confirmation !== action?.match || !action}
              onClick={() => {
                if (action) onAction?.(action.id)
                close()
              }}
            >
              <TriangleAlert />
              {action?.label}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
