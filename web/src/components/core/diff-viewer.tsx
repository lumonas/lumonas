import { useMemo } from 'react'
import { cn } from '@/lib/utils'

interface DiffRow {
  type: 'ctx' | 'add' | 'del'
  text: string
  beforeLine: number | null
  afterLine: number | null
}

function diffLines(before: string[], after: string[]): DiffRow[] {
  const n = before.length
  const m = after.length
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] =
        before[i] === after[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  const rows: DiffRow[] = []
  let i = 0
  let j = 0
  let beforeLine = 1
  let afterLine = 1
  while (i < n && j < m) {
    if (before[i] === after[j]) {
      rows.push({ type: 'ctx', text: before[i], beforeLine, afterLine })
      i++
      j++
      beforeLine++
      afterLine++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      rows.push({ type: 'del', text: before[i], beforeLine, afterLine: null })
      i++
      beforeLine++
    } else {
      rows.push({ type: 'add', text: after[j], beforeLine: null, afterLine })
      j++
      afterLine++
    }
  }
  while (i < n) {
    rows.push({ type: 'del', text: before[i], beforeLine, afterLine: null })
    i++
    beforeLine++
  }
  while (j < m) {
    rows.push({ type: 'add', text: after[j], beforeLine: null, afterLine })
    j++
    afterLine++
  }
  return rows
}

export function DiffViewer({
  before,
  after,
  titleBefore = 'Current',
  titleAfter = 'Proposed',
  showLineNumbers = true,
  className,
}: {
  before: string
  after: string
  titleBefore?: string
  titleAfter?: string
  showLineNumbers?: boolean
  className?: string
}) {
  const rows = useMemo(() => diffLines(before.split('\n'), after.split('\n')), [before, after])
  const additions = rows.filter((r) => r.type === 'add').length
  const deletions = rows.filter((r) => r.type === 'del').length
  const unchanged = rows.filter((r) => r.type === 'ctx').length

  return (
    <div className={cn('overflow-hidden rounded-lg border', className)}>
      <div className="flex items-center justify-between gap-2 border-b bg-muted/40 px-3 py-2">
        <span className="text-xs font-medium text-muted-foreground">
          {titleBefore} → {titleAfter}
        </span>
        <span className="tnum flex gap-3 text-xs">
          <span className="text-success">+{additions}</span>
          <span className="text-critical">−{deletions}</span>
          {unchanged > 0 && <span className="text-muted-foreground">{unchanged} unchanged</span>}
        </span>
      </div>
      <div className="max-h-96 overflow-auto font-mono text-xs leading-relaxed">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-center text-muted-foreground">No differences</div>
        ) : (
          rows.map((row, index) => (
            <div
              key={index}
              className={cn(
                'flex',
                row.type === 'add' && 'bg-success/8',
                row.type === 'del' && 'bg-critical/8',
                row.type === 'ctx' && 'hover:bg-muted/30',
              )}
            >
              {showLineNumbers && (
                <span className="w-10 shrink-0 select-none border-r px-1 py-px text-right text-muted-foreground/50">
                  {row.beforeLine ?? ''}
                </span>
              )}
              {showLineNumbers && (
                <span className="w-10 shrink-0 select-none border-r px-1 py-px text-right text-muted-foreground/50">
                  {row.afterLine ?? ''}
                </span>
              )}
              <span
                className={cn(
                  'w-6 shrink-0 px-1 py-px text-center',
                  row.type === 'add' && 'text-success',
                  row.type === 'del' && 'text-critical',
                  row.type === 'ctx' && 'text-muted-foreground/40',
                )}
                aria-hidden
              >
                {row.type === 'add' ? '+' : row.type === 'del' ? '−' : ' '}
              </span>
              <span
                className={cn(
                  'flex-1 whitespace-pre-wrap break-all px-2 py-px',
                  row.type === 'add' && 'text-success',
                  row.type === 'del' && 'text-critical line-through decoration-critical/30',
                  row.type === 'ctx' && 'text-muted-foreground',
                )}
              >
                {row.text || '\u00A0'}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
