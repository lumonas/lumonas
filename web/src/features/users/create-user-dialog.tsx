import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useUsers } from '@/api/queries'
import { useCreateUser } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export function CreateUserDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { data: users, refetch } = useUsers()
  const createUser = useCreateUser()
  const [type, setType] = useState<'management' | 'file'>('file')
  const [username, setUsername] = useState('')
  const [fullName, setFullName] = useState('')
  const [role, setRole] = useState<'operator' | 'readonly'>('operator')
  const [group, setGroup] = useState<string>('family')
  const [password, setPassword] = useState('')

  function close(next: boolean) {
    if (!next) {
      setUsername('')
      setFullName('')
      setPassword('')
      setType('file')
      setRole('operator')
      setGroup('family')
    }
    onOpenChange(next)
  }

  const allNames = [
    ...(users?.management ?? []).map((u) => u.username),
    ...(users?.file ?? []).map((u) => u.username),
  ]
  const usernameTaken = allNames.includes(username.trim())
  const valid =
    username.trim().length >= 2 &&
    !usernameTaken &&
    password.length >= 4 &&
    /^[a-z0-9_-]+$/.test(username.trim())

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Create user</DialogTitle>
          <DialogDescription>
            Passwords are stored encrypted in the LumoNAS secret store.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="grid gap-2">
            <Label>User type</Label>
            <Select
              value={type}
              onValueChange={(v) => setType(v as 'management' | 'file')}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="file">File user — share access only</SelectItem>
                <SelectItem value="management">Management user — signs in to LumoNAS</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="user-username">Username</Label>
            <Input
              id="user-username"
              value={username}
              onChange={(e) => setUsername(e.target.value.toLowerCase())}
              className="font-mono"
              placeholder="lowercase, no spaces"
              autoFocus
            />
            {usernameTaken && (
              <p className="text-xs text-critical">This username is already in use.</p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="user-fullname">Full name (optional)</Label>
            <Input
              id="user-fullname"
              value={fullName}
              onChange={(e) => setFullName(e.target.value)}
            />
          </div>

          {type === 'management' ? (
            <div className="grid gap-2">
              <Label>Management role</Label>
              <Select value={role} onValueChange={(v) => setRole(v as 'operator' | 'readonly')}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="operator">Operator — manage everything</SelectItem>
                  <SelectItem value="readonly">Read-only — view only</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                The first Owner account already exists and cannot be created here.
              </p>
            </div>
          ) : (
            <div className="grid gap-2">
              <Label>Group</Label>
              <Select value={group} onValueChange={setGroup}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(users?.groups ?? []).map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Share access can be granted to the whole group at once.
              </p>
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="user-password">Password</Label>
            <Input
              id="user-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
            />
            <p className="text-xs text-muted-foreground">Minimum 4 characters in this demo.</p>
          </div>

          {type === 'file' && (
            <AlertBanner tone="info" title="No management access">
              File users cannot sign in to this interface unless you create a separate management
              user.
            </AlertBanner>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => close(false)}>
            Cancel
          </Button>
          <Button
            disabled={!valid || createUser.isPending}
            onClick={() =>
              createUser.mutate(
                {
                  type,
                  username: username.trim(),
                  fullName: fullName.trim() || undefined,
                  role: type === 'management' ? role : undefined,
                  group: type === 'file' ? group : undefined,
                  password,
                },
                {
                  onSuccess: () => {
                    void refetch()
                    close(false)
                  },
                },
              )
            }
          >
            {createUser.isPending && <Loader2 className="animate-spin" />}
            Create user
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
