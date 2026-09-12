import { useState } from 'react'
import { useStackAction } from '@/api/queries'
import { OperationReview } from '@/components/core/operation-review'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { DockerStack } from '@/api/types'

export function UpdateWizard({
  stack,
  open,
  onOpenChange,
}: {
  stack: DockerStack
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const stackAction = useStackAction()
  const [backup, setBackup] = useState(true)

  const update = stack.updateAvailable
  if (!update) return null

  return (
    <OperationReview
      open={open}
      onOpenChange={onOpenChange}
      title={`Update ${stack.name}`}
      description={`Image ${update.current} → ${update.latest}. Updates are never installed automatically.`}
      steps={[
        'Snapshot stack configuration',
        backup ? 'Back up configuration (recovery bundle)' : 'Skip configuration backup',
        `Pull new image (${update.latest})`,
        'Recreate containers',
        'Health check — previous image is retained for rollback',
      ]}
      warnings={
        backup
          ? []
          : [
              {
                label: 'No backup will be taken',
                description:
                  'If the new version migrates its database, rolling back may be harder without a fresh recovery bundle.',
              },
            ]
      }
      confirmLabel="Update stack"
      loading={stackAction.isPending}
      onConfirm={() => {
        stackAction.mutate(
          { id: stack.id, action: 'update', body: { backup } },
          { onSuccess: () => onOpenChange(false) },
        )
      }}
    >
      <div className="flex items-center justify-between gap-4 rounded-lg border bg-muted/30 px-3 py-3">
        <div>
          <Label htmlFor="update-backup" className="text-sm">
            Back up configuration first
          </Label>
          <p className="text-xs text-muted-foreground">
            Runs an encrypted recovery bundle before touching the stack.
          </p>
        </div>
        <Switch id="update-backup" checked={backup} onCheckedChange={setBackup} />
      </div>
    </OperationReview>
  )
}
