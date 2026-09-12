import { ShieldCheck } from 'lucide-react'
import { useAudit } from '@/api/queries'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDateTime } from '@/lib/format'

export function AuditTab() {
  const { data: entries, isLoading } = useAudit()
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <ShieldCheck className="size-4" />
          Audit log
        </CardTitle>
        <CardDescription>Every configuration change and security-relevant action.</CardDescription>
      </CardHeader>
      <CardContent>
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
      </CardContent>
    </Card>
  )
}
