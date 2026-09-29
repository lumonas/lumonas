import { useState } from 'react'
import { useCreateShare, useShareStorageResources } from '@/api/queries'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { formatBytes } from '@/lib/format'
import type { ShareProtocolType } from '@/api/types'

const PROTOCOLS: ShareProtocolType[] = ['smb', 'nfs', 'sftp', 'rsync', 'timemachine']

export function CreateShareWizard({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const create = useCreateShare()
  const { data: resources, isLoading: resourcesLoading, isError: resourcesError } = useShareStorageResources()
  const [name, setName] = useState('')
  const [resourceChoice, setResourceChoice] = useState<string | null>(null)
  const [relativePath, setRelativePath] = useState('/')
  const [enabledProtocols, setEnabledProtocols] = useState<ShareProtocolType[]>(['smb'])
  // Derive the selection during render instead of syncing it in an effect: an
  // explicit choice wins, and a choice that no longer resolves (resources
  // reloaded, or the pool was removed) falls back to the first location.
  const resourceId =
    resourceChoice && resources?.some((item) => item.id === resourceChoice)
      ? resourceChoice
      : resources?.[0]?.id ?? ''
  const resource = resources?.find((item) => item.id === resourceId)

  function reset() {
    setName('')
    setResourceChoice(null)
    setRelativePath('/')
    setEnabledProtocols(['smb'])
  }

  function submit() {
    create.mutate(
      {
        name,
        resourceId,
        resourceLabel: resource?.label ?? resourceId,
        relativePath,
        access: [],
        protocols: PROTOCOLS.map((protocol) => ({
          protocol,
          enabled: enabledProtocols.includes(protocol),
        })),
      },
      { onSuccess: () => { reset(); onOpenChange(false) } },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create share</DialogTitle>
          <DialogDescription>Choose a mounted pool or data disk. New share files will be stored under that location.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="share-name">Name</Label>
            <Input id="share-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="documents" />
          </div>
          <div className="grid gap-2">
            <Label>Storage resource</Label>
            <Select value={resourceId} onValueChange={setResourceChoice} disabled={resourcesLoading || !resources?.length}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{resources?.map((item) => <SelectItem key={item.id} value={item.id}>{item.label} · {item.kind}</SelectItem>)}</SelectContent>
            </Select>
            {resource ? <p className="text-xs text-muted-foreground">{resource.path}{resource.totalBytes ? ` · ${formatBytes(Math.max(0, resource.totalBytes - (resource.usedBytes ?? 0)))} available` : ''}</p> : null}
            {resourcesError ? <p role="alert" className="text-xs text-destructive">Storage locations could not be loaded.</p> : null}
            {!resourcesLoading && !resourcesError && !resources?.length ? <p className="text-xs text-muted-foreground">No mounted pools or data disks are available. Mount storage before creating a share.</p> : null}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="share-relative-path">Relative path</Label>
            <Input id="share-relative-path" value={relativePath} onChange={(event) => setRelativePath(event.target.value)} placeholder="/media" />
          </div>
          <div className="grid gap-2">
            <Label>Protocols</Label>
            {PROTOCOLS.map((protocol) => (
              <div key={protocol} className="flex items-center justify-between rounded-md border px-3 py-2">
                <span className="text-sm uppercase">{protocol}</span>
                <Switch checked={enabledProtocols.includes(protocol)} onCheckedChange={(checked) => setEnabledProtocols((current) => checked ? [...new Set([...current, protocol])] : current.filter((item) => item !== protocol))} aria-label={`Enable ${protocol}`} />
              </div>
            ))}
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button disabled={!name.trim() || !resource || enabledProtocols.length === 0 || create.isPending} onClick={submit}>Create share</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
