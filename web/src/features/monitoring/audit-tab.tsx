import { Download, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { apiDownload } from '@/api/client'
import { useAudit, useAuditRetention, useSetAuditRetention } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDateTime } from '@/lib/format'

export function AuditTab() {
  const [actor, setActor] = useState('')
  const [action, setAction] = useState('')
  const [query, setQuery] = useState('')
  const [outcome, setOutcome] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [cursor, setCursor] = useState<string | undefined>()
  const [history, setHistory] = useState<Array<string | undefined>>([])
  const filters = {
    actor: actor.trim(), action: action.trim(), outcome, q: query.trim(),
    from: from ? `${from}T00:00:00.000Z` : '', to: to ? `${to}T23:59:59.999Z` : '',
  }
  const { data: page, isLoading } = useAudit(filters, cursor)
  const entries = page?.entries
  const { data: retention } = useAuditRetention()
  const updateRetention = useSetAuditRetention()
  async function exportAudit() {
    try {
      const params = new URLSearchParams()
      for (const [key, value] of Object.entries(filters)) if (value) params.set(key, value)
      const blob = await apiDownload(`/audit/export${params.size ? `?${params.toString()}` : ''}`)
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `lumonas-audit-${new Date().toISOString().slice(0, 10)}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Could not export audit log')
    }
  }
  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><ShieldCheck className="size-4" />Audit log</CardTitle>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground">Keep for
              <select aria-label="Audit log retention" className="ml-2 h-8 rounded-md border bg-background px-2 text-foreground" value={retention?.retentionDays ?? 365} disabled={updateRetention.isPending} onChange={(event) => updateRetention.mutate(Number(event.target.value))}>
                {[90, 180, 365, 730, 1825, 3650].map((days) => <option key={days} value={days}>{days >= 365 ? `${days / 365} year${days === 365 ? '' : 's'}` : `${days} days`}</option>)}
              </select>
            </label>
            <Button size="sm" variant="outline" onClick={() => void exportAudit()}><Download />Export CSV</Button>
          </div>
        </div>
        <CardDescription>Filter security history and export every matching retained entry.</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="mb-3 flex flex-wrap gap-2">
          <input aria-label="Filter by actor" className="h-8 min-w-36 rounded-md border bg-background px-2 text-xs" placeholder="Actor" value={actor} onChange={(event) => { setActor(event.target.value); setCursor(undefined); setHistory([]) }} />
          <input aria-label="Filter by action" className="h-8 min-w-40 rounded-md border bg-background px-2 text-xs" placeholder="Action" value={action} onChange={(event) => { setAction(event.target.value); setCursor(undefined); setHistory([]) }} />
          <input aria-label="Search audit log" className="h-8 min-w-48 flex-1 rounded-md border bg-background px-2 text-xs" placeholder="Search resource or metadata" value={query} onChange={(event) => { setQuery(event.target.value); setCursor(undefined); setHistory([]) }} />
          <select aria-label="Filter by outcome" className="h-8 rounded-md border bg-background px-2 text-xs" value={outcome} onChange={(event) => { setOutcome(event.target.value); setCursor(undefined); setHistory([]) }}>
            <option value="">All outcomes</option><option value="committed">Committed</option><option value="recorded">Recorded</option><option value="failed">Failed</option><option value="denied">Denied</option>
          </select>
          <input aria-label="From date" type="date" className="h-8 rounded-md border bg-background px-2 text-xs" value={from} onChange={(event) => { setFrom(event.target.value); setCursor(undefined); setHistory([]) }} />
          <input aria-label="To date" type="date" className="h-8 rounded-md border bg-background px-2 text-xs" value={to} onChange={(event) => { setTo(event.target.value); setCursor(undefined); setHistory([]) }} />
        </div>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading audit entries…</p>
        ) : (entries?.length ?? 0) === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">No audit entries yet.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Resource</TableHead>
                <TableHead>Trace</TableHead>
                <TableHead>Outcome</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries?.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDateTime(entry.timestamp)}
                  </TableCell>
                  <TableCell className="text-xs font-medium">{entry.actor}</TableCell>
                  <TableCell className="font-mono text-xs">{entry.action}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {entry.resourceType ? `${entry.resourceType}/${entry.resourceId ?? ''}` : '—'}
                  </TableCell>
                  <TableCell className="max-w-[220px] text-xs text-muted-foreground">
                    {entry.operationId || entry.correlationId ? (
                      <div className="space-y-0.5" title={[entry.operationId, entry.planHash, entry.correlationId].filter(Boolean).join(' · ')}>
                        {entry.operationId && <div className="font-mono">op: {entry.operationId}</div>}
                        {entry.planHash && <div className="font-mono">plan: {entry.planHash}</div>}
                        {entry.correlationId && <div className="font-mono">req: {entry.correlationId}</div>}
                        {entry.generation !== undefined && <div>generation: {entry.generation}</div>}
                      </div>
                    ) : '—'}
                  </TableCell>
                  <TableCell>
                    <Badge variant={entry.outcome === 'recorded' || entry.outcome === 'committed' ? 'secondary' : 'warning'}>
                      {entry.outcome}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <div className="mt-3 flex justify-end gap-2">
          <Button size="sm" variant="outline" disabled={history.length === 0 || isLoading} onClick={() => { setCursor(history.at(-1)); setHistory((items) => items.slice(0, -1)) }}>Previous</Button>
          <Button size="sm" variant="outline" disabled={!page?.hasMore || isLoading} onClick={() => { if (page?.nextCursor) { setHistory((items) => [...items, cursor]); setCursor(page.nextCursor) } }}>Next</Button>
        </div>
      </CardContent>
    </Card>
  )
}
