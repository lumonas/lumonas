import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus, ShieldCheck, ShieldOff, Users } from 'lucide-react'
import { useToggleUser, useUsers } from '@/api/queries'
import { PageHeader } from '@/components/core/page-header'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { CreateUserDialog } from '@/features/users/create-user-dialog'
import { TwoFactorDialog } from '@/features/users/twofactor-dialog'
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
  const [createOpen, setCreateOpen] = useState(searchParams.get('create') === '1')
  const [twoFactorUser, setTwoFactorUser] = useState<ManagementUser | null>(null)

  const managementColumns: Column<ManagementUser>[] = [
    {
      id: 'user',
      header: 'User',
      sortValue: (u) => u.username,
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
  ]

  const fileColumns: Column<FileUser>[] = [
    {
      id: 'user',
      header: 'User',
      sortValue: (u) => u.username,
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
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {(users?.groups ?? []).map((group) => (
                <li key={group.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{group.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {group.members.join(', ')}
                    </p>
                  </div>
                  <Badge variant="secondary">{group.members.length} members</Badge>
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
    </div>
  )
}
