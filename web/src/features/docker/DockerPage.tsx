import { useSearchParams } from 'react-router-dom'
import { PageHeader } from '@/components/core/page-header'
import { AppsTab } from '@/features/docker/apps-tab'
import { ContainersTab } from '@/features/docker/containers-tab'
import { ImagesTab } from '@/features/docker/images-tab'
import { StackDrawer } from '@/features/docker/stack-drawer'
import { StacksTab } from '@/features/docker/stacks-tab'
import { VolumesTab } from '@/features/docker/volumes-tab'
import { VirtualMachinesTab } from '@/features/docker/virtual-machines-tab'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useDockerSummary } from '@/api/queries'
import { DockerAvailabilityBanner } from '@/features/docker/docker-availability'

const TABS = [
  { value: 'apps', label: 'Apps' },
  { value: 'stacks', label: 'Stacks' },
  { value: 'containers', label: 'Containers' },
  { value: 'images', label: 'Images' },
  { value: 'volumes', label: 'Volumes' },
  { value: 'vms', label: 'Virtual machines' },
] as const

export function DockerPage() {
  const { data: docker } = useDockerSummary()
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab = tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'apps'
  const stackId = searchParams.get('stack')
  const installId = searchParams.get('install')

  function setParam(key: string, value: string | null) {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Docker"
        description="Apps, stacks and containers. Compose stays the source of truth."
      />
      <DockerAvailabilityBanner summary={docker} />
      <Tabs value={tab} onValueChange={(value) => setParam('tab', value)} className="gap-6">
        <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
          {TABS.map((t) => (
            <TabsTrigger key={t.value} value={t.value}>
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="apps">
          <AppsTab initialInstallId={installId} />
        </TabsContent>
        <TabsContent value="stacks">
          <StacksTab />
        </TabsContent>
        <TabsContent value="containers">
          <ContainersTab />
        </TabsContent>
        <TabsContent value="images">
          <ImagesTab />
        </TabsContent>
        <TabsContent value="volumes">
          <VolumesTab />
        </TabsContent>
        <TabsContent value="vms">
          <VirtualMachinesTab />
        </TabsContent>
      </Tabs>
      <StackDrawer
        stackId={stackId}
        onOpenChange={(open) => setParam('stack', open ? stackId : null)}
      />
    </div>
  )
}
