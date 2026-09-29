import { useState } from 'react'
import { FileSearch, Search } from 'lucide-react'
import { useBuildFileContentIndex, useFileContentIndexStatus, useFileContentSearch, useFileSearch } from '@/api/queries'
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
import type { FileContentResult, FileEntry } from '@/api/types'

export function FileSearchDialog({ shareId }: { shareId: string | null }) {
  const [query, setQuery] = useState('')
  const [contentMode, setContentMode] = useState(false)
  const names = useFileSearch(shareId, contentMode ? '' : query)
  const content = useFileContentSearch(shareId, contentMode ? query : '')
  const index = useFileContentIndexStatus(contentMode ? shareId : null)
  const buildIndex = useBuildFileContentIndex()
  const results: (FileEntry | FileContentResult)[] = contentMode ? content.data?.results ?? [] : names.data ?? []
  const isFetching = contentMode ? content.isFetching : names.isFetching

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
          <DialogDescription>Search names across shares, or build a private per-share text index to search inside files.</DialogDescription>
        </DialogHeader>
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="vacation-2024.jpg"
          autoFocus
        />
        <label className="flex items-center gap-2 text-xs text-muted-foreground"><input type="checkbox" checked={contentMode} onChange={(event) => setContentMode(event.target.checked)} />Search inside indexed text files (this share only)</label>
        {contentMode ? <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 p-2 text-xs text-muted-foreground"><span className="min-w-0 flex-1">{index.data?.indexedAt ? index.data.documents > 0 ? `${index.data.documents.toLocaleString()} files indexed · ${new Date(index.data.indexedAt).toLocaleString()}` : `Index built ${new Date(index.data.indexedAt).toLocaleString()} · no supported text files found.` : 'No content index yet.'} Supported text and config files up to 512 KiB each.</span><Button size="sm" variant="outline" disabled={!shareId || buildIndex.isPending} onClick={() => shareId && buildIndex.mutate(shareId)}><FileSearch />{buildIndex.isPending ? 'Indexing…' : index.data?.indexedAt ? 'Rebuild index' : 'Build index'}</Button></div> : null}
        <div className="flex max-h-72 flex-col overflow-auto rounded-md border">
          {contentMode && content.isError ? <p className="p-3 text-xs text-muted-foreground">Build the content index for this share before searching its contents.</p> : null}
          {query.trim().length >= (contentMode ? 3 : 2) && results.length === 0 && !isFetching && !content.isError ? (
            <p className="p-4 text-center text-sm text-muted-foreground">No matches.</p>
          ) : null}
          {results.map((entry) => (
            <div
              key={'path' in entry ? `${entry.shareId}:${entry.path}/${entry.name}` : entry.id}
              className="flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0"
            >
              <div className="min-w-0">
                <p className="truncate font-medium">{entry.name}</p>
                <p className="truncate text-xs text-muted-foreground">{'path' in entry ? `${entry.path}${entry.path ? '/' : ''}${entry.name}` : entry.id}</p>
                {'snippet' in entry ? <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{entry.snippet}</p> : null}
              </div>
              <div className="tnum shrink-0 text-xs text-muted-foreground">
                {'type' in entry ? entry.type === 'file' ? formatBytes(entry.sizeBytes) : 'folder' : formatBytes(entry.sizeBytes)} ·{' '}
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
