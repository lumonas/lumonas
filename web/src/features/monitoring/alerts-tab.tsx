import { useState, type FormEvent } from 'react'
import { Mail, MessageSquare, Send, Webhook, Bell } from 'lucide-react'
import {
  useAlertRules,
  useNotificationChannels,
  useSaveNotificationChannel,
  useTestNotificationChannel,
  useToggleAlertRule,
  type NotificationChannelInput,
} from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { AlertHistoryCard } from '@/features/monitoring/alert-history-card'
import { timeAgo } from '@/lib/format'
import type { AlertRule, NotificationChannel } from '@/api/types'

const SEVERITY_VARIANT: Record<AlertRule['severity'], 'info' | 'attention' | 'warning' | 'critical'> = {
  info: 'info',
  attention: 'attention',
  warning: 'warning',
  critical: 'critical',
}

const CHANNEL_ICONS: Record<NotificationChannel['type'], React.ElementType> = {
  web: Bell,
  telegram: Send,
  email: Mail,
  ntfy: MessageSquare,
  discord: MessageSquare,
  webhook: Webhook,
  slack: MessageSquare,
  gotify: Bell,
  smtp: Mail,
}

export function AlertsTab() {
  const [channelOpen, setChannelOpen] = useState(false)
  const [type, setType] = useState<NotificationChannelInput['type']>('webhook')
  const [label, setLabel] = useState('')
  const [target, setTarget] = useState('')
  const [token, setToken] = useState('')
  const { data: rules, isLoading } = useAlertRules()
  const { data: channels } = useNotificationChannels()
  const saveChannel = useSaveNotificationChannel()
  const testChannel = useTestNotificationChannel()
  const toggle = useToggleAlertRule()

  function resetChannelForm() {
    setLabel('')
    setTarget('')
    setToken('')
    setType('webhook')
  }

  function submitChannel(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    saveChannel.mutate(
      { type, label: label.trim(), target: target.trim(), enabled: true, credentials: token.trim() ? { token: token.trim() } : undefined },
      {
        onSuccess: () => {
          setChannelOpen(false)
          resetChannelForm()
        },
      },
    )
  }

  const columns: Column<AlertRule>[] = [
    {
      id: 'rule',
      header: 'Rule',
      sortValue: (r) => r.name,
      cell: (r) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{r.name}</span>
          <span className="text-xs text-muted-foreground">{r.condition}</span>
        </div>
      ),
    },
    {
      id: 'severity',
      header: 'Severity',
      sortValue: (r) => r.severity,
      cell: (r) => (
        <Badge variant={SEVERITY_VARIANT[r.severity]} className="capitalize">
          {r.severity}
        </Badge>
      ),
    },
    {
      id: 'routes',
      header: 'Routes',
      cell: (r) => (
        <span className="text-xs text-muted-foreground">
          {r.routes.map((route) => route.toUpperCase()).join(' · ')}
        </span>
      ),
    },
    {
      id: 'last',
      header: 'Last triggered',
      sortValue: (r) => r.lastTriggeredAt ?? '',
      cell: (r) => (
        <span className="text-xs text-muted-foreground">
          {r.lastTriggeredAt ? timeAgo(r.lastTriggeredAt) : 'never'}
        </span>
      ),
    },
    {
      id: 'enabled',
      header: 'Enabled',
      cell: (r) => (
        <Switch
          checked={r.enabled}
          onCheckedChange={() => toggle.mutate(r)}
          aria-label={`Toggle rule ${r.name}`}
        />
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <ResourceTable columns={columns} rows={rules ?? []} loading={isLoading} />
      <AlertHistoryCard />

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Notification channels
          </CardTitle>
          <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => setChannelOpen(true)}>
            Add channel
          </Button>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {(channels ?? []).map((channel) => {
              const Icon = CHANNEL_ICONS[channel.type]
              return (
                <li key={channel.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="flex min-w-0 items-center gap-3">
                    <Icon className="size-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{channel.label}</p>
                      {channel.target ? (
                        <p className="truncate font-mono text-xs text-muted-foreground">
                          {channel.target}
                        </p>
                      ) : channel.configured ? null : (
                        <p className="text-xs text-muted-foreground">Not configured</p>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    {channel.enabled && <Badge variant="success">Enabled</Badge>}
                    {!channel.configured && <Badge variant="outline">Setup needed</Badge>}
                    {channel.configured && !channel.enabled && (
                      <Badge variant="secondary">Disabled</Badge>
                    )}
                    {channel.configured && channel.type !== 'web' && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 text-xs"
                        disabled={testChannel.isPending}
                        onClick={() => testChannel.mutate(channel.id)}
                      >
                        Test
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
          <p className="mt-3 text-xs text-muted-foreground">
            Secrets (bot tokens, SMTP passwords) live in the LumoNAS secret store and never appear in
            exports or logs.
          </p>
        </CardContent>
      </Card>

      <Dialog open={channelOpen} onOpenChange={setChannelOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add notification channel</DialogTitle>
            <DialogDescription>
              Credentials are encrypted by the backend and are never returned to the UI.
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submitChannel}>
            <div className="space-y-2">
              <Label htmlFor="notification-channel-type">Provider</Label>
              <select
                id="notification-channel-type"
                value={type}
                onChange={(event) => setType(event.target.value as NotificationChannelInput['type'])}
                className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground"
              >
                {(['webhook', 'ntfy', 'gotify', 'telegram', 'slack', 'discord'] as const).map((value) => (
                  <option key={value} value={value}>{value}</option>
                ))}
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="notification-channel-label">Label</Label>
              <Input id="notification-channel-label" value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Home notifications" required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="notification-channel-target">Target</Label>
              <Input id="notification-channel-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder={type === 'telegram' ? 'chat ID' : 'https://example.test/hook'} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="notification-channel-token">Token (optional)</Label>
              <Input id="notification-channel-token" type="password" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="new-password" />
            </div>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setChannelOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={saveChannel.isPending || !label.trim() || !target.trim()}>Save channel</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
