import { useState, type FormEvent } from 'react'
import { Bell, Plus, Send, Trash2 } from 'lucide-react'
import {
  useDeleteNotificationChannel,
  useNotificationChannels,
  useNotificationDeliveries,
  useNotificationRules,
  useSaveNotificationChannel,
  useSaveNotificationRule,
  useTestNotificationChannel,
  useToggleNotificationRule,
	useUpdateNotificationChannel,
	type NotificationCredentials,
	type NotificationChannelInput,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import { timeAgo } from '@/lib/format'
import type { NotificationChannel } from '@/api/types'

const CHANNEL_TYPES: NotificationChannelInput['type'][] = [
  'webhook',
  'ntfy',
  'telegram',
  'slack',
  'discord',
  'gotify',
  'smtp',
]

function ChannelDialog({
  open,
  onOpenChange,
  editing,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  editing: NotificationChannel | null
}) {
  const save = useSaveNotificationChannel()
  const update = useUpdateNotificationChannel()
	const [type, setType] = useState<NotificationChannelInput['type']>(() => editing?.type && editing.type !== 'web' ? editing.type : 'webhook')
	const [label, setLabel] = useState(() => editing?.label ?? '')
	const [target, setTarget] = useState(() => editing?.target ?? '')
	const [token, setToken] = useState('')
	const [username, setUsername] = useState('')
	const [password, setPassword] = useState('')

	function reset() {
		setLabel('')
		setTarget('')
		setToken('')
		setUsername('')
		setPassword('')
	}

	function credentials(): NotificationCredentials | undefined {
		if (type === 'smtp') {
			return username || password ? { username: username || undefined, password: password || undefined } : undefined
		}
		return token ? { token } : undefined
	}

	function handleOpenChange(nextOpen: boolean) {
		if (!nextOpen) reset()
		onOpenChange(nextOpen)
	}

  function submit(event: FormEvent) {
    event.preventDefault()
    if (editing) {
      update.mutate(
        {
          id: editing.id,
          label: label || undefined,
          target: target || undefined,
			credentials: credentials(),
        },
        { onSuccess: () => { onOpenChange(false); reset() } },
      )
      return
    }
    save.mutate(
		{ type, label, target, enabled: true, credentials: credentials() },
      { onSuccess: () => { onOpenChange(false); reset() } },
    )
  }

  return (
	    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{editing ? `Edit channel — ${editing.label}` : 'Add a notification channel'}</DialogTitle>
          <DialogDescription>
            Credentials are encrypted with the recovery key and never shown again.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="grid gap-3">
          {!editing ? (
            <div className="grid gap-2">
              <Label>Type</Label>
              <div className="flex flex-wrap gap-1.5">
                {CHANNEL_TYPES.map((candidate) => (
                  <button
                    key={candidate}
                    type="button"
                    onClick={() => setType(candidate)}
                    className={`rounded-md border px-2.5 py-1 text-xs font-medium ${type === candidate ? 'border-primary bg-primary/10 text-primary' : 'text-muted-foreground'}`}
                  >
                    {candidate}
                  </button>
                ))}
              </div>
            </div>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="channel-label">Label</Label>
            <Input id="channel-label" value={label} onChange={(event) => setLabel(event.target.value)} placeholder={editing?.label ?? 'Ops Telegram'} />
          </div>
		  <div className="grid gap-2">
			<Label htmlFor="channel-target">{type === 'smtp' ? 'SMTP target' : 'Target'}</Label>
			<Input id="channel-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder={type === 'smtp' ? 'smtp://mail.example:587?to=you@example.com' : editing?.target ?? 'https://hooks.example/…'} />
		  </div>
		  {type === 'smtp' ? (
			<>
			  <div className="grid gap-2">
				<Label htmlFor="channel-username">SMTP username / sender</Label>
				<Input id="channel-username" type="email" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="nas@example.com" />
			  </div>
			  <div className="grid gap-2">
				<Label htmlFor="channel-password">SMTP password {editing ? '(leave empty to keep)' : ''}</Label>
				<Input id="channel-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="SMTP password" />
			  </div>
			</>
		  ) : (
			<div className="grid gap-2">
			  <Label htmlFor="channel-token">Credential {editing ? '(leave empty to keep)' : ''}</Label>
			  <Input id="channel-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} placeholder="token / password / key" />
			</div>
		  )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
			<Button type="submit" disabled={!label || (!editing && !target) || (!editing && type === 'smtp' && !username)}>
              {editing ? 'Save changes' : 'Add channel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ChannelsCard() {
  const { data: channels } = useNotificationChannels()
  const remove = useDeleteNotificationChannel()
  const test = useTestNotificationChannel()
  const update = useUpdateNotificationChannel()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<NotificationChannel | null>(null)

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Bell className="size-4" />
          Channels
        </CardTitle>
        <Button size="sm" onClick={() => { setEditing(null); setDialogOpen(true) }}>
          <Plus />
          Add channel
        </Button>
      </CardHeader>
	  <CardContent>
        <ul className="flex flex-col divide-y rounded-lg border">
          {(channels ?? []).map((channel) => (
            <li key={channel.id} className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5">
              <div className="min-w-0">
                <p className="flex items-center gap-2 truncate text-sm font-medium">
                  {channel.label}
                  <Badge variant="secondary">{channel.type}</Badge>
                  {!channel.configured && channel.type !== 'web' ? (
                    <Badge variant="warning">no credentials</Badge>
                  ) : null}
                </p>
                <p className="truncate text-xs text-muted-foreground">{channel.target ?? 'in-app only'}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Switch
                  checked={channel.enabled}
                  disabled={channel.type === 'web'}
                  onCheckedChange={(checked) => update.mutate({ id: channel.id, enabled: checked })}
                  aria-label={`Toggle ${channel.label}`}
                />
                {channel.type !== 'web' ? (
                  <>
                    <Button size="sm" variant="outline" className="h-7 text-xs" onClick={() => test.mutate(channel.id)} disabled={test.isPending}>
                      <Send />
                      Test
                    </Button>
                    <Button size="sm" variant="outline" className="h-7 text-xs" onClick={() => { setEditing(channel); setDialogOpen(true) }}>
                      Edit
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`Delete ${channel.label}`}
                      onClick={() => {
                        if (confirm(`Delete channel “${channel.label}”?`)) remove.mutate(channel.id)
                      }}
                    >
                      <Trash2 />
                    </Button>
                  </>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
		<ChannelDialog key={editing?.id ?? 'new'} open={dialogOpen} onOpenChange={setDialogOpen} editing={editing} />
      </CardContent>
    </Card>
  )
}

function RulesCard() {
  const { data: rules } = useNotificationRules()
  const save = useSaveNotificationRule()
  const toggle = useToggleNotificationRule()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [condition, setCondition] = useState('')

  function submit(event: FormEvent) {
    event.preventDefault()
    save.mutate(
      { name, condition, severity: 'warning', routes: ['web'] },
      { onSuccess: () => { setOpen(false); setName(''); setCondition('') } },
    )
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">Routing rules</CardTitle>
        <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
          <Plus />
          Add rule
        </Button>
      </CardHeader>
      <CardContent>
        <ul className="flex flex-col divide-y rounded-lg border">
          {(rules ?? []).map((rule) => (
            <li key={rule.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{rule.name}</p>
                <p className="truncate text-xs text-muted-foreground">{rule.condition}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {rule.lastTriggeredAt ? (
                  <span className="text-xs text-muted-foreground">last {timeAgo(rule.lastTriggeredAt)}</span>
                ) : null}
                <Badge variant={rule.severity === 'critical' ? 'critical' : rule.severity === 'warning' ? 'warning' : 'info'}>
                  {rule.severity}
                </Badge>
                <Switch
                  checked={rule.enabled}
                  onCheckedChange={() => toggle.mutate(rule)}
                  aria-label={`Toggle ${rule.name}`}
                />
              </div>
            </li>
          ))}
        </ul>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <DialogTitle>Add a routing rule</DialogTitle>
              <DialogDescription>
                Rules route matching events to channels. Conditions use the same wording as alert
                rules; matching is done by the notification engine.
              </DialogDescription>
            </DialogHeader>
            <form onSubmit={submit} className="grid gap-3">
              <div className="grid gap-2">
                <Label htmlFor="rule-name">Name</Label>
                <Input id="rule-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Page on critical" />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="rule-condition">Condition</Label>
                <Input id="rule-condition" value={condition} onChange={(event) => setCondition(event.target.value)} placeholder="severity >= warning" />
              </div>
              <DialogFooter>
                <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
                <Button type="submit" disabled={!name || !condition}>Save rule</Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}

function DeliveriesCard() {
  const { data: deliveries } = useNotificationDeliveries()
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">Recent deliveries</CardTitle>
        <CardDescription>Every notification attempt with its outcome.</CardDescription>
      </CardHeader>
      <CardContent>
        {(deliveries?.length ?? 0) === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">No deliveries recorded yet.</p>
        ) : (
          <ul className="flex flex-col divide-y rounded-lg border">
            {deliveries?.map((delivery) => (
              <li key={delivery.id} className="flex items-center justify-between gap-3 px-3 py-2">
                <div className="min-w-0">
                  <p className="truncate font-mono text-xs">{delivery.eventType}</p>
                  {delivery.error ? (
                    <p className="truncate text-xs text-critical">{delivery.error}</p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <span className="text-xs text-muted-foreground">{timeAgo(delivery.attemptedAt)}</span>
                  <Badge variant={delivery.state === 'sent' ? 'success' : delivery.state === 'suppressed' ? 'secondary' : 'critical'}>
                    {delivery.state}
                  </Badge>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function NotificationsTab() {
  const test = useTestNotificationChannel()
  if (test.isError) {
    return (
      <div className="flex flex-col gap-4">
        <AlertBanner tone="critical" title="Test delivery failed">
          {test.error instanceof Error ? test.error.message : null}
        </AlertBanner>
        <ChannelsCard />
        <RulesCard />
        <DeliveriesCard />
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <ChannelsCard />
      <RulesCard />
      <DeliveriesCard />
    </div>
  )
}
