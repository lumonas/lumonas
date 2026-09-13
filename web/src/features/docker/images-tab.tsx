import { useRef, useState } from 'react'
import { ArrowUpCircle, FileUp, PackageOpen, RefreshCw } from 'lucide-react'
import {
  useCheckImageUpdates,
  useDockerImagePacks,
  useDockerImages,
  useImagePackImport,
  useImportDockerImage,
  useUpdateImage,
} from '@/api/queries'
import { EmptyState } from '@/components/core/empty-state'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { formatBytes } from '@/lib/format'
import type { DockerImage } from '@/api/types'

export function ImagesTab() {
  const { data: images, isLoading } = useDockerImages()
  const { data: packs } = useDockerImagePacks()
  const updateImage = useUpdateImage()
  const importImage = useImportDockerImage()
  const importPack = useImagePackImport()
  const checkUpdates = useCheckImageUpdates()
  const fileInput = useRef<HTMLInputElement>(null)
  const [importOpen, setImportOpen] = useState(false)

  const columns: Column<DockerImage>[] = [
    {
      id: 'image',
      header: 'Image',
      sortValue: (i) => `${i.repo}:${i.tag}`,
      cell: (i) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] font-medium">{i.repo}</span>
          <span className="font-mono text-xs text-muted-foreground">:{i.tag}</span>
        </div>
      ),
    },
    {
      id: 'size',
      header: 'Size',
      className: 'tnum',
      sortValue: (i) => i.sizeBytes,
      cell: (i) => formatBytes(i.sizeBytes),
    },
    {
      id: 'pulled',
      header: 'Pulled',
      className: 'tnum',
      sortValue: (i) => i.createdDaysAgo,
      cell: (i) => <span className="text-muted-foreground">{i.createdDaysAgo}d ago</span>,
    },
    {
      id: 'inUse',
      header: 'In use',
      sortValue: (i) => (i.inUse ? 0 : 1),
      cell: (i) =>
        i.inUse ? (
          <Badge variant="secondary">In use</Badge>
        ) : (
          <Badge variant="outline" className="text-muted-foreground">
            Unused
          </Badge>
        ),
    },
    {
      id: 'update',
      header: 'Update',
      cell: (i) =>
        i.updateAvailable ? (
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1.5 text-xs"
            disabled={updateImage.isPending}
            onClick={() => updateImage.mutate(i.id)}
          >
            <ArrowUpCircle className="text-primary" />
            Pull newer
          </Button>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        ),
    },
  ]

  return (
    <>
      <ResourceTable
        columns={columns}
        rows={images ?? []}
        loading={isLoading}
        emptyState={
          <EmptyState
            icon={<FileUp />}
            title="No images"
            description="Pulled images will appear here."
            className="border-0"
          />
        }
      />
      {packs && packs.length > 0 ? (
        <div className="rounded-xl border p-4">
          <div className="flex items-center gap-2">
            <PackageOpen className="size-4 text-muted-foreground" />
            <h3 className="text-sm font-semibold">Offline image packs</h3>
            <span className="text-xs text-muted-foreground">Verified bundles for air-gapped installs</span>
          </div>
          <div className="mt-3 flex flex-col gap-2">
            {packs.map((pack) => (
              <div key={pack.name} className="flex items-center justify-between gap-3 rounded-lg border bg-muted/20 px-3 py-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {pack.name}
                    {pack.version ? <span className="ml-2 font-mono text-xs text-muted-foreground">{pack.version}</span> : null}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {pack.imageCount} image{pack.imageCount === 1 ? '' : 's'} · {formatBytes(pack.totalSizeBytes)}
                    {pack.description ? ` · ${pack.description}` : ''}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 shrink-0 gap-1.5 text-xs"
                  disabled={importPack.isPending}
                  onClick={() => importPack.mutate(pack.name)}
                >
                  {importPack.isPending ? 'Importing…' : 'Import pack'}
                </Button>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      <div className="flex justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => checkUpdates.mutate(undefined)}
          disabled={checkUpdates.isPending}
        >
          <RefreshCw className={checkUpdates.isPending ? 'animate-spin' : undefined} />
          {checkUpdates.isPending ? 'Checking…' : 'Check for updates'}
        </Button>
        <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
          <FileUp />
          Import image…
        </Button>
      </div>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Import image</DialogTitle>
            <DialogDescription>
              Import a <span className="font-mono">docker save</span> tar archive — useful when the
              NAS is offline.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-8 text-center">
            <FileUp className="size-6 text-muted-foreground" />
            <p className="text-sm font-medium">Choose a .tar archive</p>
            <p className="text-xs text-muted-foreground">
              Produced by <span className="font-mono">docker save</span> on another machine
            </p>
            <input
              ref={fileInput}
              type="file"
              accept=".tar,application/x-tar,application/octet-stream"
              className="sr-only"
              onChange={(event) => {
                const archive = event.target.files?.[0]
                if (!archive) return
                importImage.mutate(archive, { onSuccess: () => setImportOpen(false) })
                event.target.value = ''
              }}
            />
            <Button variant="outline" onClick={() => fileInput.current?.click()} disabled={importImage.isPending}>
              {importImage.isPending ? 'Importing…' : 'Choose archive'}
            </Button>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setImportOpen(false)}>
              Cancel
            </Button>
            <Button variant="outline" onClick={() => fileInput.current?.click()} disabled={importImage.isPending}>
              {importImage.isPending ? 'Importing…' : 'Import'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
