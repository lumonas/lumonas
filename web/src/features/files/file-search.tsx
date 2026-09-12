import { useState } from 'react'
import { Search } from 'lucide-react'
import { useFileSearch } from '@/api/queries'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { formatBytes, timeAgo } from '@/lib/format'
import type { FileEntry } from '@/api/types'

export function FileSearchDialog() {
  const [query, setQuery] = useState('')
  const { data: results, isFetching } = useFileSearch(query)

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <Search />
          Search
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Search files</DialogTitle>
          <DialogDescription>Searches all shares by name — at least two characters.</DialogDescription>
        </DialogHeader>
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="vacation-2024.jpg"
          autoFocus
        />
        <div className="flex max-h-72 flex-col overflow-auto rounded-md border">
          {query.trim().length >= 2 && (results?.length ?? 0) === 0 && !isFetching ? (
            <p className="p-4 text-center text-sm text-muted-foreground">No matches.</p>
          ) : null}
          {(results ?? []).map((entry: FileEntry) => (
            <div
              key={entry.id}
              className="flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0"
            >
              <div className="min-w-0">
                <p className="truncate font-medium">{entry.name}</p>
                <p className="truncate text-xs text-muted-foreground">{entry.id}</p>
              </div>
              <div className="tnum shrink-0 text-xs text-muted-foreground">
                {entry.type === 'file' ? formatBytes(entry.sizeBytes) : 'folder'} ·{' '}
                {timeAgo(entry.modifiedAt)}
              </div>
            </div>
          ))}
          {isFetching ? (
            <p className="p-4 text-center text-sm text-muted-foreground">Searching…</p>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}
