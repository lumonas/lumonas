import { useState } from 'react'
import { KeyRound, Trash2 } from 'lucide-react'
import { useSSHKeys, useAddSSHKey, useRemoveSSHKey } from '@/api/queries'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SSHKeysDialog() {
  const { data: keys } = useSSHKeys()
  const addKey = useAddSSHKey()
  const removeKey = useRemoveSSHKey()
  const [newKey, setNewKey] = useState('')
  const [open, setOpen] = useState(false)

  function handleAdd() {
    if (!newKey.trim()) return
    addKey.mutate(newKey.trim(), {
      onSuccess: () => setNewKey(''),
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="h-7 text-xs">
          Manage
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="size-4" />
            SSH Authorized Keys
          </DialogTitle>
          <DialogDescription>
            Manage public keys allowed to authenticate as root. Keys are stored in ~/.ssh/authorized_keys.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {keys && keys.length > 0 ? (
            <ul className="flex flex-col divide-y rounded-lg border">
              {keys.map((key) => (
                <li key={key.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-xs">{key.publicKey}</p>
                    {key.comment && (
                      <p className="text-xs text-muted-foreground">{key.comment}</p>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="shrink-0"
                    disabled={removeKey.isPending}
                    onClick={() => removeKey.mutate(key.publicKey)}
                  >
                    <Trash2 className="size-3.5 text-muted-foreground" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">No SSH keys configured.</p>
          )}

          <div className="space-y-2">
            <Label htmlFor="new-ssh-key">Add new key</Label>
            <div className="flex gap-2">
              <Input
                id="new-ssh-key"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value)}
                placeholder="ssh-ed25519 AAAA... user@host"
                className="font-mono text-xs"
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleAdd()
                }}
              />
              <Button
                onClick={handleAdd}
                disabled={!newKey.trim() || addKey.isPending}
                className="shrink-0"
              >
                {addKey.isPending ? 'Adding...' : 'Add'}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
