import { useSearchParams } from 'react-router-dom'
import { PageHeader } from '@/components/core/page-header'
import { DiskDrawer } from '@/features/storage/disk-drawer'
import { DisksTab } from '@/features/storage/disks-tab'
import { OverviewTab } from '@/features/storage/overview-tab'
import {
  PoolsTab,
  ProtectionTab,
  StorageActivityTab,
} from '@/features/storage/storage-tabs'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'disks', label: 'Disks' },
  { value: 'pools', label: 'Pools' },
  { value: 'protection', label: 'Protection' },
  { value: 'activity', label: 'Activity' },
] as const

export function StoragePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab =
    tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'overview'
  const diskId = searchParams.get('disk')

  function setParam(key: string, value: string | null) {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Storage"
        description="Disks, pools and SnapRAID parity protection."
      />
      <Tabs
        value={tab}
        onValueChange={(value) => setParam('tab', value)}
        className="gap-6"
      >
        <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
          {TABS.map((t) => (
            <TabsTrigger key={t.value} value={t.value}>
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview">
          <OverviewTab onBrowseDisks={() => setParam('tab', 'disks')} />
        </TabsContent>
        <TabsContent value="disks">
          <DisksTab />
        </TabsContent>
        <TabsContent value="pools">
          <PoolsTab />
        </TabsContent>
        <TabsContent value="protection">
          <ProtectionTab />
        </TabsContent>
        <TabsContent value="activity">
          <StorageActivityTab />
        </TabsContent>
      </Tabs>
      <DiskDrawer diskId={diskId} onOpenChange={(open) => setParam('disk', open ? diskId : null)} />
    </div>
  )
}
