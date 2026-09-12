import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, ShieldCheck, UserRound, UsersRound } from 'lucide-react'
import { apiGet, apiPatch, apiPost } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

type Principal = {
  id: string
  kind: 'user' | 'group' | 'service'
  name: string
  uid?: number
  gid?: number
  managementRole: 'none' | 'owner' | 'admin' | 'operator' | 'readonly'
  enabled: boolean
  groups?: string[]
}

const roleLabels = { none: 'File access only', owner: 'Owner', admin: 'Administrator', operator: 'Operator', readonly: 'Read-only' }

export function UsersPage() {
  const queryClient = useQueryClient()
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: () => apiGet<Principal[]>('/users') })
  const groups = useQuery({ queryKey: ['admin', 'groups'], queryFn: () => apiGet<Principal[]>('/groups') })
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Principal['managementRole']>('none')
  const [groupName, setGroupName] = useState('')
  const [message, setMessage] = useState('')

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['admin'] })
  }
  const createUser = useMutation({
    mutationFn: () => apiPost<Principal>('/users', { name, password, managementRole: role, kind: 'user' }),
    onSuccess: () => { setName(''); setPassword(''); setMessage('User created.'); refresh() },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to create user.'),
  })
  const createGroup = useMutation({
    mutationFn: () => apiPost<Principal>('/groups', { name: groupName }),
    onSuccess: () => { setGroupName(''); setMessage('Group created.'); refresh() },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to create group.'),
  })
  const toggleUser = useMutation({
    mutationFn: (user: Principal) => apiPatch<Principal>(`/users/${user.id}`, { enabled: !user.enabled }),
    onSuccess: () => { setMessage('Account status updated.'); refresh() },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to update account.'),
  })

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Users" description="Manage management identities, file users, groups, and access roles." />
      <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><UserRound className="size-4 text-primary" />Users and service identities</CardTitle>
            <CardDescription>Disabled identities cannot sign in or access new sessions.</CardDescription>
          </CardHeader>
          <CardContent>
            {users.isLoading ? <p className="text-sm text-muted-foreground">Loading users…</p> : users.isError ? <p className="text-sm text-critical">Unable to load users. Check the API connection.</p> : (
              <Table>
                <TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Type</TableHead><TableHead>Role</TableHead><TableHead>Status</TableHead><TableHead /></TableRow></TableHeader>
                <TableBody>
                  {users.data?.map((user) => <TableRow key={user.id}>
                    <TableCell className="font-medium">{user.name}</TableCell>
                    <TableCell className="capitalize">{user.kind}</TableCell>
                    <TableCell>{roleLabels[user.managementRole]}</TableCell>
                    <TableCell><Badge variant={user.enabled ? 'success' : 'offline'}>{user.enabled ? 'Enabled' : 'Disabled'}</Badge></TableCell>
                    <TableCell><Button size="sm" variant="outline" onClick={() => toggleUser.mutate(user)}>{user.enabled ? 'Disable' : 'Enable'}</Button></TableCell>
                  </TableRow>)}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2"><Plus className="size-4 text-primary" />Add user</CardTitle><CardDescription>Management roles require a strong password.</CardDescription></CardHeader>
            <CardContent className="space-y-3">
              <Label htmlFor="user-name">Username</Label><Input id="user-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="alex" />
              <Label htmlFor="user-password">Password</Label><Input id="user-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="At least 12 characters for admins" />
              <Label>Management role</Label>
              <Select value={role} onValueChange={(value) => setRole(value as Principal['managementRole'])}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{Object.entries(roleLabels).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}</SelectContent></Select>
              <Button className="w-full" disabled={!name || createUser.isPending} onClick={() => createUser.mutate()}><ShieldCheck />Create user</Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2"><UsersRound className="size-4 text-primary" />Groups</CardTitle><CardDescription>Groups are the stable unit for share permissions.</CardDescription></CardHeader>
            <CardContent className="space-y-3">
              <div className="flex gap-2"><Input value={groupName} onChange={(event) => setGroupName(event.target.value)} placeholder="family" /><Button disabled={!groupName || createGroup.isPending} onClick={() => createGroup.mutate()}>Add</Button></div>
              <div className="flex flex-wrap gap-2">{groups.data?.map((group) => <Badge key={group.id} variant="secondary">{group.name} · {group.groups?.length ?? 0} members</Badge>)}</div>
            </CardContent>
          </Card>
        </div>
      </div>
      {message ? <p className="text-sm text-muted-foreground" role="status"><KeyRound className="mr-1 inline size-3" />{message}</p> : null}
    </div>
  )
}
