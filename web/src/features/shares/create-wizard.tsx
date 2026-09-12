import { useState } from 'react'
import { useCreateShare } from '@/api/queries'
import { STORAGE_RESOURCES } from '@/api/resources'
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
  const [name, setName] = useState('')
  const [resourceId, setResourceId] = useState(STORAGE_RESOURCES[0]?.id ?? '')
  const [relativePath, setRelativePath] = useState('/')
  const [enabledProtocols, setEnabledProtocols] = useState<ShareProtocolType[]>(['smb'])
  const resource = STORAGE_RESOURCES.find((item) => item.id === resourceId)

  function reset() {
    setName('')
    setResourceId(STORAGE_RESOURCES[0]?.id ?? '')
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
          <DialogDescription>Publish a storage location through one or more validated protocols.</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="share-name">Name</Label>
            <Input id="share-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="documents" />
          </div>
          <div className="grid gap-2">
            <Label>Storage resource</Label>
            <Select value={resourceId} onValueChange={setResourceId}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{STORAGE_RESOURCES.map((item) => <SelectItem key={item.id} value={item.id}>{item.label}</SelectItem>)}</SelectContent>
            </Select>
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
          <Button disabled={!name.trim() || enabledProtocols.length === 0 || create.isPending} onClick={submit}>Create share</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
