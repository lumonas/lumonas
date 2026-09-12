import { useState } from 'react'
import { Loader2 } from 'lucide-react'
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
import { formatBytes, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { FileEntry, Share } from '@/api/types'

export function MkdirDialog({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (name: string) => void
}) {
  const [name, setName] = useState('')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>New folder</DialogTitle>
          <DialogDescription>Created inside the current folder.</DialogDescription>
        </DialogHeader>
        <Input
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Folder name"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && name.trim()) {
              onCreate(name.trim())
              setName('')
            }
          }}
        />
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!name.trim()}
            onClick={() => {
              onCreate(name.trim())
              setName('')
            }}
          >
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function RenameDialog({
  entry,
  onOpenChange,
  onRename,
}: {
  entry: FileEntry | null
  onOpenChange: (open: boolean) => void
  onRename: (newName: string) => void
}) {
  const [name, setName] = useState('')
  const value = entry && name === '' ? entry.name : name
  return (
    <Dialog
      open={entry != null}
      onOpenChange={(next) => {
        if (!next) setName('')
        onOpenChange(next)
      }}
    >
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename</DialogTitle>
        </DialogHeader>
        <Input
          autoFocus
          value={value}
          onChange={(e) => setName(e.target.value)}
          onFocus={(e) => {
            const dot = e.target.value.lastIndexOf('.')
            e.target.setSelectionRange(0, dot > 0 ? dot : e.target.value.length)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && value.trim() && entry && value.trim() !== entry.name) {
              onRename(value.trim())
              setName('')
            }
          }}
        />
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!value.trim() || (entry != null && value.trim() === entry.name)}
            onClick={() => {
              if (entry) onRename(value.trim())
              setName('')
            }}
          >
            Rename
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function DeleteDialog({
  names,
  onOpenChange,
  onDelete,
}: {
  names: string[] | null
  onOpenChange: (open: boolean) => void
  onDelete: () => void
}) {
  return (
    <Dialog open={names != null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            Delete {names?.length === 1 ? '“' + names[0] + '”' : `${names?.length ?? 0} items`}?
          </DialogTitle>
          <DialogDescription>
            Items are moved to the recycle bin and kept for 30 days.
          </DialogDescription>
        </DialogHeader>
        {names && names.length > 0 && (
          <ul className="max-h-32 overflow-auto rounded-lg border bg-muted/20 p-2 font-mono text-xs text-muted-foreground">
            {names.map((name) => (
              <li key={name} className="truncate">
                {name}
              </li>
            ))}
          </ul>
        )}
        <p className="text-xs text-muted-foreground">
          Note for SnapRAID: deletions count as unsynced changes — the Protection screen will
          reflect them until the next parity sync.
        </p>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={onDelete}>
            Move to recycle bin
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function PropertiesDialog({
  entry,
  share,
  onOpenChange,
}: {
  entry: FileEntry | null
  share: Share | undefined
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={entry != null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Properties</DialogTitle>
        </DialogHeader>
        {entry && (
          <div className="flex flex-col divide-y rounded-lg border text-sm">
            <PropRow label="Name" value={entry.name} mono />
            <PropRow label="Type" value={entry.type === 'dir' ? 'Folder' : 'File'} />
            <PropRow
              label="Size"
              value={entry.type === 'dir' ? `${formatBytes(entry.sizeBytes)} (contents)` : formatBytes(entry.sizeBytes)}
            />
            <PropRow label="Modified" value={formatDateTime(entry.modifiedAt)} />
            <PropRow
              label="Location"
              value={`${share?.name ?? ''} ${share?.relativePath ?? ''}`.trim()}
              mono
            />
          </div>
        )}
        {share && (
          <div>
            <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Effective access (via share)
            </p>
            <ul className="flex flex-col divide-y rounded-lg border">
              {share.access
                .filter((a) => a.level !== 'none')
                .map((a) => (
                  <li key={a.principalId} className="flex items-center justify-between px-3 py-2 text-sm">
                    <span>{a.principalId.replace('p-', '')}</span>
                    <span className="text-xs text-muted-foreground">
                      {a.level === 'write' ? 'Read & write' : 'Read only'}
                    </span>
                  </li>
                ))}
            </ul>
            <p className="mt-2 text-xs text-muted-foreground">
              One-off permission changes in a file browser never silently conflict with managed
              share ACLs.
            </p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function PropRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn('min-w-0 truncate text-right', mono && 'font-mono text-xs')}>{value}</span>
    </div>
  )
}

export type ConflictMode = 'overwrite' | 'skip' | 'rename'

export function ConflictDialog({
  conflicts,
  pending,
  onResolve,
  onOpenChange,
}: {
  conflicts: string[] | null
  pending: boolean
  onResolve: (mode: ConflictMode) => void
  onOpenChange: (open: boolean) => void
}) {
  const [mode, setMode] = useState<ConflictMode>('rename')
  return (
    <Dialog open={conflicts != null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Items already exist</DialogTitle>
          <DialogDescription>
            {conflicts?.length === 1
              ? `“${conflicts[0]}” already exists in the target folder.`
              : `${conflicts?.length ?? 0} items already exist in the target folder.`}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          {(
            [
              { value: 'rename', label: 'Keep both', hint: 'Adds a number, e.g. “file (2)”' },
              { value: 'overwrite', label: 'Overwrite', hint: 'Replaces existing items' },
              { value: 'skip', label: 'Skip', hint: 'Leaves existing items untouched' },
            ] as const
          ).map((option) => (
            <label
              key={option.value}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors',
                mode === option.value && 'border-primary/50 bg-primary/5',
              )}
            >
              <input
                type="radio"
                name="conflict-mode"
                className="mt-0.5 accent-primary"
                checked={mode === option.value}
                onChange={() => setMode(option.value)}
              />
              <span>
                <span className="block text-sm font-medium">{option.label}</span>
                <span className="block text-xs text-muted-foreground">{option.hint}</span>
              </span>
            </label>
          ))}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={pending} onClick={() => onResolve(mode)}>
            {pending && <Loader2 className="animate-spin" />}
            Continue
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
