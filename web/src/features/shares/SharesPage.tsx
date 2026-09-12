import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { FolderOpen, Plus } from 'lucide-react'
import { useShares } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { EmptyState } from '@/components/core/empty-state'
import { PageHeader } from '@/components/core/page-header'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ShareDrawer } from '@/features/shares/share-drawer'
import { CreateShareWizard } from '@/features/shares/create-wizard'
import { formatBytes } from '@/lib/format'
import type { Share } from '@/api/types'

const PROTOCOL_ORDER = ['smb', 'nfs', 'sftp', 'rsync', 'timemachine'] as const

export function SharesPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { data: shares, isLoading } = useShares()
  const [wizardOpen, setWizardOpen] = useState(searchParams.get('create') === '1')

  const shareId = searchParams.get('share')

  function setParam(key: string, value: string | null) {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: true })
  }

  const columns: Column<Share>[] = [
    {
      id: 'name',
      header: 'Share',
      sortValue: (s) => s.name,
      cell: (s) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{s.name}</span>
          {s.description ? (
            <span className="truncate text-xs text-muted-foreground">{s.description}</span>
          ) : null}
        </div>
      ),
    },
    {
      id: 'location',
      header: 'Location',
      sortValue: (s) => `${s.resourceLabel}${s.relativePath}`,
      cell: (s) => (
        <div className="flex flex-col">
          <span className="text-[13px]">{s.resourceLabel}</span>
          <span className="font-mono text-xs text-muted-foreground">{s.relativePath}</span>
        </div>
      ),
    },
    {
      id: 'protocols',
      header: 'Protocols',
      cell: (s) => (
        <div className="flex flex-wrap gap-1">
          {[...s.protocols]
            .filter((p) => p.enabled)
            .sort((a, b) => PROTOCOL_ORDER.indexOf(a.protocol) - PROTOCOL_ORDER.indexOf(b.protocol))
            .map((p) => (
              <Badge key={p.protocol} variant="secondary" className="uppercase">
                {p.protocol === 'timemachine' ? 'TM' : p.protocol}
              </Badge>
            ))}
        </div>
      ),
    },
    {
      id: 'access',
      header: 'Access',
      cell: (s) => (
        <span className="text-xs text-muted-foreground">
          {s.access.filter((a) => a.level !== 'none').length} principals
        </span>
      ),
    },
    {
      id: 'used',
      header: 'Used',
      advanced: true,
      className: 'tnum',
      sortValue: (s) => s.usedBytes ?? 0,
      cell: (s) => (s.usedBytes != null ? formatBytes(s.usedBytes) : '—'),
    },
    {
      id: 'status',
      header: 'Status',
      sortValue: (s) => s.status,
      cell: (s) => <HealthBadge state={s.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Shares"
        description="One share can expose one location over multiple protocols."
        actions={
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus />
            Create share
          </Button>
        }
      />
      <ResourceTable
        columns={columns}
        rows={shares ?? []}
        loading={isLoading}
        onRowClick={(share) => navigate(`/shares?share=${share.id}`)}
        emptyState={
          <EmptyState
            icon={<FolderOpen />}
            title="No shares"
            description="Create your first share to make storage available on the network."
            className="border-0"
          />
        }
      />

      <ShareDrawer
        shareId={shareId}
        onOpenChange={(open) => setParam('share', open ? shareId : null)}
      />
      <CreateShareWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
      />
    </div>
  )
}
