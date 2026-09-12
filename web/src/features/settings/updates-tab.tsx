import { useNavigate } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import { useCheckUpdates, useSettings, useUpdateSettings } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'

export function UpdatesTab() {
  const navigate = useNavigate()
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  const checkUpdates = useCheckUpdates()

  if (!settings) return null
  const { updates } = settings

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Nothing updates silently — every update runs through a verified backup first and keeps a
          rollback point.
        </p>
        <Button
          size="sm"
          variant="outline"
          disabled={checkUpdates.isPending}
          onClick={() => checkUpdates.mutate()}
        >
          <RefreshCw className={cn(checkUpdates.isPending && 'animate-spin')} />
          Check all channels
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            LumoNAS core
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <span className="tnum text-lg font-semibold">{updates.core.current}</span>
            {updates.core.available ? (
              <Badge variant="attention">Update available — {updates.core.available}</Badge>
            ) : (
              <Badge variant="success">Up to date</Badge>
            )}
            <span className="text-xs text-muted-foreground">
              checked {timeAgo(updates.core.lastCheckedAt)}
            </span>
            <span className="flex-1" />
            <Button
              size="sm"
              disabled={!updates.core.available}
              onClick={() => navigate('/updates')}
            >
              Open update manager
            </Button>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-4 border-t pt-3">
            <Label htmlFor="core-channel" className="text-xs text-muted-foreground">
              Channel
            </Label>
            <div className="flex items-center gap-4">
              <Select
                value={updates.core.channel}
                onValueChange={(channel) =>
                  updateSettings.mutate({
                    section: 'updates',
                    patch: { core: { ...updates.core, channel } },
                  })
                }
              >
                <SelectTrigger id="core-channel" className="h-8 w-32 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="stable">Stable</SelectItem>
                  <SelectItem value="beta">Beta</SelectItem>
                </SelectContent>
              </Select>
              <div className="flex items-center gap-2">
                <Label htmlFor="core-auto" className="text-xs text-muted-foreground">
                  Auto-update
                </Label>
                <Switch
                  id="core-auto"
                  checked={updates.core.autoUpdate}
                  onCheckedChange={(next) =>
                    updateSettings.mutate({
                      section: 'updates',
                      patch: { core: { ...updates.core, autoUpdate: next } },
                    })
                  }
                />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Debian security updates
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm font-medium">{updates.debian.release}</span>
            {updates.debian.pendingCount > 0 ? (
              <Badge variant="attention">
                {updates.debian.pendingCount} security packages pending
              </Badge>
            ) : (
              <Badge variant="success">All applied</Badge>
            )}
            <span className="text-xs text-muted-foreground">
              checked {timeAgo(updates.debian.lastCheckedAt)}
            </span>
            <span className="flex-1" />
            <Button
              size="sm"
              variant="outline"
              disabled={updates.debian.pendingCount === 0}
              onClick={() => navigate('/updates')}
            >
              Open update manager
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Security patches apply without reboot where possible; a required reboot is always
            scheduled explicitly by you.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Docker images
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            {updates.docker.availableCount > 0 ? (
              <Badge variant="attention">
                {updates.docker.availableCount} image update
                {updates.docker.availableCount === 1 ? '' : 's'} available
              </Badge>
            ) : (
              <Badge variant="success">All current</Badge>
            )}
            <span className="text-xs text-muted-foreground">
              Updated per-app with rollback — never blindly.
            </span>
          </div>
          <a
            href="/docker?tab=images"
            className="text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Review in Docker
          </a>
        </CardContent>
      </Card>

      {updates.core.autoUpdate && (
        <AlertBanner tone="warning" title="Auto-update is enabled for LumoNAS core">
          Updates still wait for a healthy recovery readiness score before applying.
        </AlertBanner>
      )}

      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3 p-4">
          <div>
            <p className="text-sm font-medium">Advanced: A/B slot updates</p>
            <p className="text-xs text-muted-foreground">
              Stage signed packages into an inactive slot and confirm health before switching.
            </p>
          </div>
          <a
            href="/updates"
            className="text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Open slot manager
          </a>
        </CardContent>
      </Card>
    </div>
  )
}
