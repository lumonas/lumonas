import { useState } from 'react'
import { useStackAction } from '@/api/queries'
import { OperationReview } from '@/components/core/operation-review'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { formatBytes, timeAgo } from '@/lib/format'
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
        backup ? 'Back up appdata (stack will be stopped briefly)' : 'Skip appdata backup',
        `Pull new image (${update.latest})`,
        'Recreate containers',
        'Health check — previous image is retained for rollback',
      ]}
      warnings={
        backup
          ? []
          : [
              {
                label: 'Appdata will not be backed up',
                description:
                  'If the new version migrates its database, rolling back may be harder without a backup.',
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
            Back up appdata first
          </Label>
          <p className="text-xs text-muted-foreground">
            {formatBytes(stack.backup.appdataSizeBytes)}
            {stack.backup.lastBackupAt ? ` · last backup ${timeAgo(stack.backup.lastBackupAt)}` : ''}
          </p>
        </div>
        <Switch id="update-backup" checked={backup} onCheckedChange={setBackup} />
      </div>
    </OperationReview>
  )
}
