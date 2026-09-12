import { useSearchParams } from 'react-router-dom'
import { PageHeader } from '@/components/core/page-header'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { BackupsOverviewTab } from '@/features/backups/overview-tab'
import { BackupJobsTab } from '@/features/backups/jobs-tab'
import { DestinationsTab } from '@/features/backups/destinations-tab'
import { HistoryTab } from '@/features/backups/history-tab'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'jobs', label: 'Jobs' },
  { value: 'history', label: 'History' },
  { value: 'destinations', label: 'Destinations' },
] as const

export function BackupsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab = tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'overview'

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Backups"
        description="Backup jobs and disaster recovery are two different things — both live here."
      />
      <Tabs
        value={tab}
        onValueChange={(value) => {
          const next = new URLSearchParams(searchParams)
          next.set('tab', value)
          setSearchParams(next, { replace: true })
        }}
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
          <BackupsOverviewTab />
        </TabsContent>
        <TabsContent value="jobs">
          <BackupJobsTab />
        </TabsContent>
        <TabsContent value="history">
          <HistoryTab />
        </TabsContent>
        <TabsContent value="destinations">
          <DestinationsTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
