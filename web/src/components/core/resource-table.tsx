import { useId, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ChevronsUpDown, Search } from 'lucide-react'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export interface Column<T> {
  id: string
  header: string
  cell: (row: T) => React.ReactNode
  sortValue?: (row: T) => string | number
  advanced?: boolean
  className?: string
  searchValue?: (row: T) => string
}

export function ResourceTable<T extends { id: string }>({
  columns,
  rows,
  onRowClick,
  loading = false,
  emptyState,
  className,
  pageSize = 50,
}: {
  columns: Column<T>[]
  rows: T[]
  onRowClick?: (row: T) => void
  loading?: boolean
  emptyState?: React.ReactNode
  className?: string
  pageSize?: number
}) {
  const [advanced, setAdvanced] = useState(false)
  const [sort, setSort] = useState<{ id: string; dir: 1 | -1 } | null>(null)
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)
  const advancedId = useId()

  const hasAdvanced = columns.some((c) => c.advanced)
  const visibleColumns = columns.filter((c) => !c.advanced || advanced)

  const filteredRows = useMemo(() => {
    const query = search.trim().toLocaleLowerCase()
    if (!query) return rows
    return rows.filter((row) => columns.some((column) => (column.searchValue?.(row) ?? '').toLocaleLowerCase().includes(query)))
  }, [rows, columns, search])

  const sortedRows = useMemo(() => {
    if (!sort) return filteredRows
    const column = columns.find((c) => c.id === sort.id)
    if (!column?.sortValue) return filteredRows
    return [...filteredRows].sort((a, b) => {
      const av = column.sortValue!(a)
      const bv = column.sortValue!(b)
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * sort.dir
      return String(av).localeCompare(String(bv)) * sort.dir
    })
  }, [filteredRows, sort, columns])

  const totalPages = Math.max(1, Math.ceil(sortedRows.length / pageSize))
  const currentPage = Math.min(page, totalPages - 1)
  const visibleRows = sortedRows.slice(currentPage * pageSize, (currentPage + 1) * pageSize)

  function toggleSort(id: string) {
    setPage(0)
    setSort((current) => {
      if (current?.id !== id) return { id, dir: 1 }
      if (current.dir === 1) return { id, dir: -1 }
      return null
    })
  }

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      {columns.some((column) => column.searchValue) && (
        <div className="relative w-full sm:max-w-xs">
          <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            value={search}
            onChange={(event) => { setSearch(event.target.value); setPage(0) }}
            placeholder="Filter this list…"
            aria-label="Filter this list"
            className="pl-8"
          />
        </div>
      )}
      {search.trim() && <p className="sr-only" aria-live="polite">{sortedRows.length} matching records</p>}
      {hasAdvanced && (
        <div className="flex items-center justify-end gap-2">
          <label htmlFor={advancedId} className="cursor-pointer text-xs text-muted-foreground select-none">
            Advanced columns
          </label>
          <Switch
            id={advancedId}
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
                  <th key={column.id} className={cn('px-3 py-2.5 text-left', column.className)} aria-sort={sort?.id === column.id ? (sort.dir === 1 ? 'ascending' : 'descending') : undefined}>
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
              visibleRows.map((row) => (
                <tr
                  key={row.id}
                  tabIndex={onRowClick ? 0 : undefined}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                  onKeyDown={
                    onRowClick
                      ? (e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault()
                            onRowClick(row)
                          }
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
      {!loading && sortedRows.length > pageSize && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
          <span>{currentPage * pageSize + 1}–{Math.min((currentPage + 1) * pageSize, sortedRows.length)} of {sortedRows.length}</span>
          <div className="flex gap-2">
            <button type="button" className="rounded border px-2 py-1 disabled:opacity-40" disabled={currentPage === 0} onClick={() => setPage((current) => Math.max(0, current - 1))}>Previous</button>
            <span className="self-center">Page {currentPage + 1} of {totalPages}</span>
            <button type="button" className="rounded border px-2 py-1 disabled:opacity-40" disabled={currentPage + 1 >= totalPages} onClick={() => setPage((current) => Math.min(totalPages - 1, current + 1))}>Next</button>
          </div>
        </div>
      )}
    </div>
  )
}
