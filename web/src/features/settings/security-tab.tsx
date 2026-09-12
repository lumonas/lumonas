import { KeyRound, Lock, ShieldCheck } from 'lucide-react'
import { useSettings, useUpdateSettings } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { SSHKeysDialog } from '@/features/settings/ssh-keys-dialog'
import { timeAgo } from '@/lib/format'

export function SecurityTab() {
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  if (!settings) return null
  const { security } = settings

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <ShieldCheck className="size-4" />
              HTTPS
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm">Enabled — local certificate</p>
                <p className="text-xs text-muted-foreground">
                  Issued by “{security.https.ca}”. Browsers show a one-time trust prompt; import
                  the CA to remove it.
                </p>
              </div>
              <Switch
                checked={security.https.enabled}
                onCheckedChange={(next) =>
                  updateSettings.mutate({
                    section: 'security',
                    patch: { https: { ...security.https, enabled: next } },
                  })
                }
                aria-label="Toggle HTTPS"
              />
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div>
                <p className="text-sm">ACME (Let's Encrypt)</p>
                <p className="text-xs text-muted-foreground">
                  Only possible with a public domain and open port 80 — off by default.
                </p>
              </div>
              <Switch
                checked={security.https.acme}
                onCheckedChange={(next) =>
                  updateSettings.mutate({
                    section: 'security',
                    patch: { https: { ...security.https, acme: next } },
                  })
                }
                aria-label="Toggle ACME"
              />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Lock className="size-4" />
              SSH
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm">Root login</p>
                <p className="text-xs text-muted-foreground">Disabled and locked (recommended).</p>
              </div>
              <Badge variant="success">Locked</Badge>
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div>
                <p className="text-sm">Password authentication</p>
                <p className="text-xs text-muted-foreground">
                  Key-only is enforced while at least one admin key exists.
                </p>
              </div>
              <Switch
                checked={security.ssh.passwordAuth}
                onCheckedChange={(next) =>
                  updateSettings.mutate({
                    section: 'security',
                    patch: { ssh: { ...security.ssh, passwordAuth: next } },
                  })
                }
                aria-label="Toggle SSH password authentication"
              />
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div className="flex items-center gap-2 text-sm">
                <KeyRound className="size-4 text-muted-foreground" />
                Authorized keys
              </div>
              <div className="flex items-center gap-2">
                <Badge variant="secondary">{security.ssh.keyCount} key</Badge>
                <SSHKeysDialog />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Active sessions
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {security.sessions.map((session) => (
              <li
                key={session.id}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5"
              >
                <div className="min-w-0">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    {session.device}
                    {session.current && <Badge variant="secondary">this device</Badge>}
                  </p>
                  <p className="tnum text-xs text-muted-foreground">
                    {session.ip} · {session.scope} · active {timeAgo(session.lastActiveAt)}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 text-xs"
                  disabled={session.current}
                  onClick={() =>
                    updateSettings.mutate({
                      section: 'security',
                      patch: {
                        sessions: security.sessions.filter((s) => s.id !== session.id),
                      },
                    })
                  }
                >
                  Revoke
                </Button>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-muted-foreground">
            Revoking a session signs that device out immediately. Failed sign-in attempts appear in
            the audit log and can trigger alerts.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
