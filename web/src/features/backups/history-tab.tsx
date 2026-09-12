import { useMemo, useState } from 'react'
import { useGenerations } from '@/api/queries'
import { DiffViewer } from '@/components/core/diff-viewer'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatDateTime } from '@/lib/format'

export function HistoryTab() {
  const { data: generations } = useGenerations()
  const committed = useMemo(
    () => (generations ?? []).filter((g) => g.status === 'committed'),
    [generations],
  )
  const [a, setA] = useState<number | null>(null)
  const [b, setB] = useState<number | null>(null)

  const genA = committed.find((g) => g.id === (a ?? committed[1]?.id)) ?? committed[1]
  const genB = committed.find((g) => g.id === (b ?? committed[0]?.id)) ?? committed[0]

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Configuration generations
          </CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {(generations ?? []).map((gen) => (
              <li key={gen.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div className="min-w-0">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <span className="tnum">Gen {gen.id}</span>
                    {gen.status === 'failed' && <Badge variant="critical">failed</Badge>}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">{gen.summary}</p>
                </div>
                <div className="shrink-0 text-right">
                  <p className="text-xs text-muted-foreground">{formatDateTime(gen.createdAt)}</p>
                  <p className="text-xs text-muted-foreground">{gen.actor}</p>
                </div>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-muted-foreground">
            Retention: 20 config generations · 30 daily · 12 monthly snapshots.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Compare generations
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <Select
              value={String(genA?.id ?? '')}
              onValueChange={(v) => setA(Number(v))}
            >
              <SelectTrigger className="h-8 w-28 text-xs">
                <SelectValue placeholder="From" />
              </SelectTrigger>
              <SelectContent>
                {committed.map((gen) => (
                  <SelectItem key={gen.id} value={String(gen.id)}>
                    Gen {gen.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span className="text-xs text-muted-foreground">→</span>
            <Select
              value={String(genB?.id ?? '')}
              onValueChange={(v) => setB(Number(v))}
            >
              <SelectTrigger className="h-8 w-28 text-xs">
                <SelectValue placeholder="To" />
              </SelectTrigger>
              <SelectContent>
                {committed.map((gen) => (
                  <SelectItem key={gen.id} value={String(gen.id)}>
                    Gen {gen.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {genA && genB ? (
            <DiffViewer
              before={genA.config}
              after={genB.config}
              titleBefore={`Gen ${genA.id}`}
              titleAfter={`Gen ${genB.id}`}
            />
          ) : (
            <p className="text-sm text-muted-foreground">Pick two generations to compare.</p>
          )}
          <p className="text-xs text-muted-foreground">
            Generations are semantic snapshots — restore selectively or roll the whole config back.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
