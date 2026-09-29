import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { KeyRound, Plus, ShieldCheck, ShieldOff, Trash2, Users } from 'lucide-react'
import {
  useCreateGroup,
  useDeleteGroup,
  useDeleteUser,
  usePrincipals,
  useSetGroupMembers,
  useSetUserPassword,
  useToggleUser,
  useUsers,
} from '@/api/queries'
import { PageHeader } from '@/components/core/page-header'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { Switch } from '@/components/ui/switch'
import { CreateUserDialog } from '@/features/users/create-user-dialog'
import { TwoFactorDialog } from '@/features/users/twofactor-dialog'
import { toast } from 'sonner'
import { timeAgo } from '@/lib/format'
import type { FileUser, ManagementUser } from '@/api/types'

const ROLE_BADGES: Record<ManagementUser['role'], { label: string; variant: 'default' | 'secondary' | 'info' }> = {
  owner: { label: 'Owner', variant: 'default' },
  operator: { label: 'Operator', variant: 'info' },
  readonly: { label: 'Read-only', variant: 'secondary' },
}

export function UsersPage() {
  const [searchParams] = useSearchParams()
  const { data: users, isLoading } = useUsers()
  const toggleUser = useToggleUser()
  const createGroup = useCreateGroup()
  const deleteGroup = useDeleteGroup()
  const setMembers = useSetGroupMembers()
  const setUserPassword = useSetUserPassword()
  const deleteUser = useDeleteUser()
  const { data: principals } = usePrincipals()
  const [createOpen, setCreateOpen] = useState(searchParams.get('create') === '1')
  const [twoFactorUser, setTwoFactorUser] = useState<ManagementUser | null>(null)
  const [passwordUser, setPasswordUser] = useState<ManagementUser | null>(null)
  const [password, setPassword] = useState('')
  const [deleteUserTarget, setDeleteUserTarget] = useState<ManagementUser | null>(null)
  const [groupName, setGroupName] = useState('')
  const [membersTarget, setMembersTarget] = useState<string | null>(null)

  const managementColumns: Column<ManagementUser>[] = [
    {
      id: 'user',
      header: 'User',
      sortValue: (u) => u.username,
      searchValue: (u) => `${u.username} ${u.fullName ?? ''} ${u.role} ${u.twoFactor ? 'two factor' : ''}`,
      cell: (u) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{u.username}</span>
          {u.fullName ? (
            <span className="text-xs text-muted-foreground">{u.fullName}</span>
          ) : null}
        </div>
      ),
    },
    {
      id: 'role',
      header: 'Role',
      sortValue: (u) => u.role,
      cell: (u) => <Badge variant={ROLE_BADGES[u.role].variant}>{ROLE_BADGES[u.role].label}</Badge>,
    },
    {
      id: '2fa',
      header: '2FA',
      sortValue: (u) => (u.twoFactor ? 0 : 1),
      cell: (u) =>
        u.twoFactor ? (
          <button
            type="button"
            onClick={() => setTwoFactorUser(u)}
            className="flex items-center gap-1 text-xs text-success"
          >
            <ShieldCheck className="size-3.5" />
            Enabled
          </button>
        ) : (
          <button
            type="button"
            onClick={() => setTwoFactorUser(u)}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
          >
            <ShieldOff className="size-3.5" />
            Off
          </button>
        ),
    },
    {
      id: 'login',
      header: 'Last login',
      sortValue: (u) => u.lastLoginAt ?? '',
      cell: (u) => (
        <span className="text-xs text-muted-foreground">
          {u.lastLoginAt ? timeAgo(u.lastLoginAt) : 'never'}
        </span>
      ),
    },
    {
      id: 'enabled',
      header: 'Enabled',
      cell: (u) => (
        <Switch
          checked={u.enabled}
          onCheckedChange={(next) => toggleUser.mutate({ id: u.id, enabled: next })}
          aria-label={`Toggle ${u.username}`}
        />
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: (u) => (
        <div className="flex justify-end gap-1">
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={`Set password for ${u.username}`}
            onClick={() => { setPasswordUser(u); setPassword('') }}
          >
            <KeyRound />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={`Delete ${u.username}`}
            onClick={() => setDeleteUserTarget(u)}
          >
            <Trash2 />
          </Button>
        </div>
      ),
    },
  ]

  const fileColumns: Column<FileUser>[] = [
    {
      id: 'user',
      header: 'User',
      sortValue: (u) => u.username,
      searchValue: (u) => `${u.username} ${u.fullName ?? ''} ${u.type} ${u.groups.join(' ')}`,
      cell: (u) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{u.username}</span>
          {u.fullName ? (
            <span className="text-xs text-muted-foreground">{u.fullName}</span>
          ) : null}
        </div>
      ),
    },
    {
      id: 'type',
      header: 'Type',
      sortValue: (u) => u.type,
      cell: (u) =>
        u.type === 'service' ? (
          <Badge variant="info">Service</Badge>
        ) : (
          <span className="text-xs text-muted-foreground">File user</span>
        ),
    },
    {
      id: 'groups',
      header: 'Groups',
      cell: (u) => (
        <div className="flex flex-wrap gap-1">
          {u.groups.length > 0 ? (
            u.groups.map((group) => (
              <Badge key={group} variant="secondary">
                {group}
              </Badge>
            ))
          ) : (
            <span className="text-xs text-muted-foreground">—</span>
          )}
        </div>
      ),
    },
    {
      id: 'uid',
      header: 'UID',
      advanced: true,
      className: 'tnum',
      sortValue: (u) => u.uid ?? 0,
      cell: (u) => u.uid ?? '—',
    },
    {
      id: 'enabled',
      header: 'Enabled',
      cell: (u) => (
        <Switch
          checked={u.enabled}
          onCheckedChange={(next) => toggleUser.mutate({ id: u.id, enabled: next })}
          aria-label={`Toggle ${u.username}`}
        />
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Users"
        description="Management access and file identities are kept separate."
        actions={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus />
            Create user
          </Button>
        }
      />

      <div className="flex flex-col gap-2">
        <h2 className="text-sm font-medium text-muted-foreground">Management users</h2>
        <ResourceTable columns={managementColumns} rows={users?.management ?? []} loading={isLoading} />
      </div>

      <div className="flex flex-col gap-2">
        <h2 className="text-sm font-medium text-muted-foreground">File users</h2>
        <ResourceTable columns={fileColumns} rows={users?.file ?? []} />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Users className="size-4" />
              Groups
            </CardTitle>
            <form
              className="flex items-center gap-1.5"
              onSubmit={(event) => {
                event.preventDefault()
                if (!groupName.trim()) return
                createGroup.mutate(groupName.trim(), {
                  onSuccess: () => setGroupName(''),
                })
              }}
            >
              <Input
                value={groupName}
                onChange={(event) => setGroupName(event.target.value)}
                placeholder="new-group"
                className="h-8 w-36 text-xs"
                aria-label="New group name"
              />
              <Button type="submit" size="sm" variant="outline" className="h-8 text-xs" disabled={!groupName.trim() || createGroup.isPending}>
                <Plus />
                Create
              </Button>
            </form>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {(users?.groups ?? []).map((group) => (
                <li key={group.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{group.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {group.members.join(', ') || 'No members'}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <Badge variant="secondary">{group.members.length} members</Badge>
                    <Button size="sm" variant="outline" className="h-7 text-xs" onClick={() => setMembersTarget(group.id)}>
                      Members
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`Delete group ${group.name}`}
                      onClick={() => {
                        if (confirm(`Delete group “${group.name}”?`)) deleteGroup.mutate(group.id)
                      }}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              About identities
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm text-muted-foreground">
            <p>
              Management users sign in to this interface. File users access shares over SMB/NFS —
              a file user does not get management access and vice versa.
            </p>
            <p>
              Service identities are used by Docker apps (e.g. Nextcloud writing to a share)
              without a human password.
            </p>
            <p>UIDs/GIDs are managed automatically; the canonical ACL model stays simple.</p>
          </CardContent>
        </Card>
      </div>

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
      <TwoFactorDialog user={twoFactorUser} open={twoFactorUser !== null} onOpenChange={(open) => { if (!open) setTwoFactorUser(null) }} />

      <Dialog open={passwordUser != null} onOpenChange={(open) => { if (!open) { setPasswordUser(null); setPassword('') } }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Set password — {passwordUser?.username}</DialogTitle>
            <DialogDescription>Minimum 12 characters. The hash is stored, never the password.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="user-password">New password</Label>
            <Input
              id="user-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="new-password"
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => { setPasswordUser(null); setPassword('') }}>Cancel</Button>
            <Button
              disabled={password.length < 12 || setUserPassword.isPending}
              onClick={() => {
                if (!passwordUser) return
                setUserPassword.mutate(
                  { id: passwordUser.id, password },
                  { onSuccess: () => { setPasswordUser(null); setPassword('') } },
                )
              }}
            >
              Save password
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteUserTarget != null} onOpenChange={(open) => { if (!open) setDeleteUserTarget(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete {deleteUserTarget?.username}?</DialogTitle>
            <DialogDescription>
              This removes the identity. Principals referenced by access rules or group membership
              cannot be deleted, and the last enabled management user is protected.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleteUserTarget(null)}>Cancel</Button>
            <Button
              variant="destructive"
              disabled={deleteUser.isPending}
              onClick={() => {
                if (!deleteUserTarget) return
                deleteUser.mutate(deleteUserTarget.id, {
                  onSuccess: () => {
                    toast.success(`Deleted — ${deleteUserTarget.username}`)
                    setDeleteUserTarget(null)
                  },
                })
              }}
            >
              {deleteUser.isPending ? 'Deleting…' : 'Delete user'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={membersTarget != null} onOpenChange={(open) => { if (!open) setMembersTarget(null) }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Members of {users?.groups.find((g) => g.id === membersTarget)?.name}</DialogTitle>
            <DialogDescription>Members are file users and services; groups cannot nest.</DialogDescription>
          </DialogHeader>
          <div className="flex max-h-64 flex-col gap-1 overflow-auto rounded-md border p-2">
            {(principals ?? [])
              .filter((principal) => principal.type !== 'group')
              .map((principal) => {
                const group = users?.groups.find((candidate) => candidate.id === membersTarget)
                const isMember = group?.members.includes(principal.name) ?? false
                return (
                  <label key={principal.id} className="flex items-center gap-2 rounded px-1.5 py-1 text-sm hover:bg-muted/40">
                    <input
                      type="checkbox"
                      className="size-4 accent-primary"
                      checked={isMember}
                      onChange={() => {
                        if (!membersTarget || !group) return
                        const memberIds = isMember
                          ? (group.members.filter((name) => name !== principal.name))
                          : [...group.members, principal.name]
                        const ids = memberIds
                          .map((name) => (principals ?? []).find((candidate) => candidate.name === name)?.id)
                          .filter((id): id is string => id != null)
                        setMembers.mutate({ id: membersTarget, memberIds: ids })
                      }}
                    />
                    <span className="truncate">{principal.name}</span>
                    <span className="ml-auto text-xs text-muted-foreground">{principal.type}</span>
                  </label>
                )
              })}
          </div>
          <DialogFooter>
            <Button onClick={() => setMembersTarget(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
