import { useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ChevronsUpDown } from 'lucide-react'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface Column<T> {
  id: string
  header: string
  cell: (row: T) => React.ReactNode
  sortValue?: (row: T) => string | number
  advanced?: boolean
  className?: string
}

export function ResourceTable<T extends { id: string }>({
  columns,
  rows,
  onRowClick,
  loading = false,
  emptyState,
  className,
}: {
  columns: Column<T>[]
  rows: T[]
  onRowClick?: (row: T) => void
  loading?: boolean
  emptyState?: React.ReactNode
  className?: string
}) {
  const [advanced, setAdvanced] = useState(false)
  const [sort, setSort] = useState<{ id: string; dir: 1 | -1 } | null>(null)

  const hasAdvanced = columns.some((c) => c.advanced)
  const visibleColumns = columns.filter((c) => !c.advanced || advanced)

  const sortedRows = useMemo(() => {
    if (!sort) return rows
    const column = columns.find((c) => c.id === sort.id)
    if (!column?.sortValue) return rows
    return [...rows].sort((a, b) => {
      const av = column.sortValue!(a)
      const bv = column.sortValue!(b)
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * sort.dir
      return String(av).localeCompare(String(bv)) * sort.dir
    })
  }, [rows, sort, columns])

  function toggleSort(id: string) {
    setSort((current) => {
      if (current?.id !== id) return { id, dir: 1 }
      if (current.dir === 1) return { id, dir: -1 }
      return null
    })
  }

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      {hasAdvanced && (
        <div className="flex items-center justify-end gap-2">
          <label htmlFor="advanced-columns" className="cursor-pointer text-xs text-muted-foreground select-none">
            Advanced columns
          </label>
          <Switch
            id="advanced-columns"
            checked={advanced}
            onCheckedChange={setAdvanced}
            aria-label="Show advanced columns"
          />
        </div>
      )}
      <div className="overflow-hidden rounded-xl border bg-card">
        <table className="w-full caption-bottom text-sm">
          <thead>
            <tr className="border-b bg-muted/40">
              {visibleColumns.map((column) => {
                const sortable = column.sortValue != null
                return (
                  <th key={column.id} className={cn('px-3 py-2.5 text-left', column.className)}>
                    {sortable ? (
                      <button
                        type="button"
                        onClick={() => toggleSort(column.id)}
                        className="inline-flex cursor-pointer items-center gap-1 text-xs font-medium tracking-wide text-muted-foreground uppercase outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        {column.header}
                        {sort?.id === column.id ? (
                          sort.dir === 1 ? (
                            <ArrowUp className="size-3" />
                          ) : (
                            <ArrowDown className="size-3" />
                          )
                        ) : (
                          <ChevronsUpDown className="size-3 opacity-50" />
                        )}
                      </button>
                    ) : (
                      <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                        {column.header}
                      </span>
                    )}
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <tr key={i} className="border-b last:border-b-0">
                  {visibleColumns.map((column) => (
                    <td key={column.id} className="px-3 py-3">
                      <Skeleton className="h-4 w-20" />
                    </td>
                  ))}
                </tr>
              ))
            ) : sortedRows.length === 0 ? (
              <tr>
                <td colSpan={visibleColumns.length} className="p-0">
                  {emptyState}
                </td>
              </tr>
            ) : (
              sortedRows.map((row) => (
                <tr
                  key={row.id}
                  tabIndex={onRowClick ? 0 : undefined}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                  onKeyDown={
                    onRowClick
                      ? (e) => {
                          if (e.key === 'Enter') onRowClick(row)
                        }
                      : undefined
                  }
                  className={cn(
                    'border-b transition-colors last:border-b-0',
                    onRowClick && 'cursor-pointer hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none',
                  )}
                >
                  {visibleColumns.map((column) => (
                    <td key={column.id} className={cn('px-3 py-2.5 align-middle', column.className)}>
                      {column.cell(row)}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
