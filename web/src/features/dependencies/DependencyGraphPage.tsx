import { ArrowRight, Network } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useDependencyGraph } from '@/api/queries'
import { PageHeader } from '@/components/core/page-header'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDateTime } from '@/lib/format'

export function DependencyGraphPage() {
  const { data, isLoading } = useDependencyGraph()
  const labels = new Map((data?.nodes ?? []).map((node) => [node.id, node]))
  return <div className="flex flex-col gap-6">
    <PageHeader title="Relationships" description="Understand what depends on each disk, pool, share, app and backup destination." />
    {isLoading ? <p className="text-sm text-muted-foreground">Building dependency graph…</p> : data ? <>
      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3"><CardTitle className="flex items-center gap-2 text-sm"><Network className="size-4" />Impact map</CardTitle><span className="text-xs text-muted-foreground">Generated {formatDateTime(data.generatedAt)}</span></CardHeader>
        <CardContent className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {data.nodes.map((node) => <Link key={node.id} to={nodePath(node)} className="rounded-lg border p-3 hover:bg-accent"><div className="flex items-center justify-between gap-2"><span className="truncate text-sm font-medium">{node.label}</span><Badge variant={node.status === 'healthy' ? 'success' : node.status === 'critical' ? 'critical' : 'attention'}>{node.type}</Badge></div><p className="mt-1 truncate font-mono text-[11px] text-muted-foreground">{node.id}</p></Link>)}
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-3"><CardTitle className="text-sm">Dependency paths</CardTitle></CardHeader>
        <CardContent className="flex flex-col divide-y rounded-lg border">
          {data.edges.map((edge) => <div key={`${edge.from}-${edge.to}-${edge.relationship}`} className="flex items-center gap-2 px-3 py-2.5 text-sm"><span className="min-w-0 flex-1 truncate">{labels.get(edge.from)?.label ?? edge.from}</span><ArrowRight className="size-4 shrink-0 text-muted-foreground" /><span className="min-w-0 flex-1 truncate">{labels.get(edge.to)?.label ?? edge.to}</span><span className="shrink-0 text-xs text-muted-foreground">{edge.relationship}</span></div>)}
          {data.edges.length === 0 ? <p className="px-3 py-4 text-sm text-muted-foreground">No relationships detected yet.</p> : null}
        </CardContent>
      </Card>
    </> : null}
  </div>
}

function nodePath(node: { type: string; id: string }): string {
  const id = node.id.split(':').slice(1).join(':')
  if (node.type === 'disk') return `/storage?tab=disks&disk=${encodeURIComponent(id)}`
  if (node.type === 'pool') return '/storage?tab=pools'
  if (node.type === 'share') return `/shares?share=${encodeURIComponent(id)}`
  if (node.type === 'stack') return `/docker?stack=${encodeURIComponent(id)}`
  if (node.type === 'backup' || node.type === 'recovery') return '/backups?tab=recovery'
  return '/'
}
