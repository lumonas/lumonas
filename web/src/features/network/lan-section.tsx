import { useState } from 'react'
import { Pencil, Power, RefreshCw } from 'lucide-react'
import { useLanHosts, useLanRename, useLanScan, useLanWake } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { timeAgo } from '@/lib/format'

export function LanHostsSection() {
  const { data: hosts } = useLanHosts()
  const scan = useLanScan()
  const wake = useLanWake()
  const rename = useLanRename()
  const [editing, setEditing] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  // Captured once per component instance: staleness is a coarse hint, not a
  // live countdown, and a render-time Date.now() would be impure.
  const [now] = useState(() => Date.now())

  const staleAfter = 7 * 24 * 3_600_000

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-4">
        <CardTitle className="text-sm font-medium text-muted-foreground">LAN devices</CardTitle>
        <Button
          size="sm"
          variant="outline"
          className="h-7 gap-1.5 text-xs"
          disabled={scan.isPending}
          onClick={() => scan.mutate()}
        >
          <RefreshCw className={scan.isPending ? 'size-3.5 animate-spin' : 'size-3.5'} />
          {scan.isPending ? 'Scanning…' : 'Scan now'}
        </Button>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {hosts && hosts.length > 0 ? (
          hosts.map((host) => {
            const key = `${host.mac}/${host.interface}`
            const recent = now - new Date(host.lastSeen).getTime() < staleAfter
            return (
              <div key={key} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
                <div className="min-w-0">
                  {editing === key ? (
                    <div className="flex items-center gap-2">
                      <Input
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        className="h-7 w-44 font-mono text-xs"
                        placeholder="hostname"
                        aria-label="Hostname"
                      />
                      <Button
                        size="sm"
                        variant="outline"
                        className="h-7 text-xs"
                        onClick={() => {
                          rename.mutate({ mac: host.mac, interface: host.interface, hostname: draft })
                          setEditing(null)
                        }}
                      >
                        Save
                      </Button>
                    </div>
                  ) : (
                    <p className="flex items-center gap-2 truncate text-sm font-medium">
                      {host.hostname || host.ip || host.mac}
                      <button
                        type="button"
                        aria-label="Rename device"
                        className="text-muted-foreground hover:text-foreground"
                        onClick={() => {
                          setEditing(key)
                          setDraft(host.hostname ?? '')
                        }}
                      >
                        <Pencil className="size-3" />
                      </button>
                      {!recent && (
                        <Badge variant="outline" className="text-[10px] text-muted-foreground">
                          offline?
                        </Badge>
                      )}
                    </p>
                  )}
                  <p className="truncate font-mono text-xs text-muted-foreground">
                    {host.mac} · {host.ip || '?'} · {host.interface} · seen {timeAgo(host.lastSeen)}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 shrink-0 gap-1.5 text-xs"
                  disabled={wake.isPending}
                  onClick={() => wake.mutate({ mac: host.mac, interface: host.interface })}
                >
                  <Power className="size-3.5" />
                  Wake
                </Button>
              </div>
            )
          })
        ) : (
          <p className="text-sm text-muted-foreground">
            No devices discovered yet. LumoNAS samples the kernel neighbor table periodically —
            run a scan to populate the list.
          </p>
        )}
        {wake.isError ? (
          <p className="text-xs text-destructive" role="alert">
            Wake failed: {String(wake.error)}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
