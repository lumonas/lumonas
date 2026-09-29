import { ExternalLink, Lightbulb, Wrench } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useTroubleshooting } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function TroubleshootingTab() {
  const { data, isLoading } = useTroubleshooting()
  if (isLoading) return <p className="text-sm text-muted-foreground">Checking system health…</p>
  if (!data || data.issues.length === 0) {
    return <AlertBanner tone="info" title="No active issues">LumoNAS did not find anything requiring action right now.</AlertBanner>
  }
  return (
    <div className="flex flex-col gap-4">
      {data.issues.map((issue) => (
        <Card key={issue.id}>
          <CardHeader className="flex-row items-start justify-between gap-3 space-y-0 pb-3">
            <div className="flex items-start gap-2.5">
              <Wrench className="mt-0.5 size-4 text-info" />
              <div><CardTitle className="text-sm">{issue.title}</CardTitle><p className="mt-1 text-xs text-muted-foreground">{issue.summary}</p></div>
            </div>
            <Badge variant={issue.severity === 'critical' ? 'critical' : issue.severity === 'warning' ? 'warning' : 'attention'}>{issue.severity}</Badge>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            {issue.cause ? <p className="rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground"><span className="font-medium text-foreground">Likely cause: </span>{issue.cause}</p> : null}
            <div>
              <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground"><Lightbulb className="size-3.5" />Recommended steps</p>
              <ol className="list-decimal space-y-1 pl-5 text-sm">{issue.steps.map((step) => <li key={step}>{step}</li>)}</ol>
            </div>
            {issue.links?.length ? <div className="flex flex-wrap gap-2">{issue.links.map((link) => <Link key={link.path} to={link.path} className="inline-flex items-center gap-1 text-sm text-primary hover:underline">{link.label}<ExternalLink className="size-3" /></Link>)}</div> : null}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
