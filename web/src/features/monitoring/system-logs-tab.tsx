import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiGet } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

type Entry = { timestamp: string; priority: string; unit: string; message: string }
type Response = { entries: Entry[]; unit: string; query: string }
const services = ['', 'lumonasd.service', 'lumonas-web.service', 'smbd.service', 'nfs-server.service', 'docker.service', 'ssh.service']

export function SystemLogsTab() {
  const [unit, setUnit] = useState('')
  const [source, setSource] = useState('')
  const [query, setQuery] = useState('')
  const [since, setSince] = useState('')
  const [limit, setLimit] = useState(200)
  const params = new URLSearchParams({ limit: String(limit) })
  if (unit) params.set('unit', unit)
  if (source) params.set('source', source)
  if (query.trim()) params.set('q', query.trim())
  if (since) params.set('since', since)
  const logs = useQuery({
    queryKey: ['system-logs', unit, source, query, since, limit],
    queryFn: () => apiGet<Response>(`/system/logs?${params.toString()}`),
    refetchInterval: 15_000,
  })
  return <Card>
    <CardHeader className="pb-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">System and service logs</CardTitle>
        <Button size="sm" variant="outline" onClick={() => void logs.refetch()} disabled={logs.isFetching}>Refresh</Button>
      </div>
      <CardDescription>Search recent journal entries. Results refresh every 15 seconds.</CardDescription>
    </CardHeader>
    <CardContent>
      <div className="mb-3 flex flex-wrap gap-2">
        <select aria-label="Filter logs by service" value={source || unit} onChange={(event) => { const value = event.target.value; setSource(value === 'smb-audit' ? value : ''); setUnit(value === 'smb-audit' ? '' : value) }} className="h-8 rounded-md border bg-background px-2 text-xs">
          <option value="">All services</option><option value="smb-audit">SMB file activity</option>{services.filter(Boolean).map((service) => <option key={service} value={service}>{service}</option>)}
        </select>
        <input aria-label="Search system logs" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search messages" className="h-8 min-w-48 flex-1 rounded-md border bg-background px-2 text-xs" />
        <select aria-label="Number of log entries" value={limit} onChange={(event) => setLimit(Number(event.target.value))} className="h-8 rounded-md border bg-background px-2 text-xs">
          {[100, 200, 500].map((count) => <option key={count} value={count}>Latest {count}</option>)}
        </select>
        <select aria-label="Log time range" value={since} onChange={(event) => setSince(event.target.value)} className="h-8 rounded-md border bg-background px-2 text-xs">
          <option value="">Any time</option><option value="1 hour ago">Last hour</option><option value="24 hours ago">Last 24 hours</option><option value="7 days ago">Last 7 days</option>
        </select>
      </div>
      {logs.isLoading ? <p className="py-6 text-center text-sm text-muted-foreground">Loading system logs…</p>
        : logs.isError ? <div role="alert" className="py-6 text-center text-sm text-destructive">System journal is unavailable. Check that the daemon can read system logs.</div>
          : (logs.data?.entries.length ?? 0) === 0 ? <p className="py-6 text-center text-sm text-muted-foreground">No entries match these filters.</p>
            : <div className="max-h-[560px] overflow-auto rounded-md border">
              <table className="w-full text-left text-xs"><thead className="sticky top-0 bg-muted/90 text-muted-foreground"><tr><th className="p-2">Time</th><th className="p-2">Service</th><th className="p-2">Priority</th><th className="p-2">Message</th></tr></thead>
                <tbody>{logs.data?.entries.map((entry, index) => <tr className="border-t align-top" key={`${entry.timestamp}-${index}`}><td className="whitespace-nowrap p-2 font-mono text-muted-foreground">{entry.timestamp}</td><td className="whitespace-nowrap p-2">{entry.unit || 'system'}</td><td className="p-2">{entry.priority || '—'}</td><td className="break-all p-2 font-mono">{entry.message}</td></tr>)}</tbody>
              </table>
            </div>}
    </CardContent>
  </Card>
}
