import { useSearchParams } from 'react-router-dom'
import { PageHeader } from '@/components/core/page-header'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AlertsTab } from '@/features/monitoring/alerts-tab'
import { AuditTab } from '@/features/monitoring/audit-tab'
import { JobsTab } from '@/features/monitoring/jobs-tab'
import { MonitoringOverviewTab } from '@/features/monitoring/overview-tab'
import { NotificationsTab } from '@/features/monitoring/notifications-tab'
import { TimelineTab } from '@/features/monitoring/timeline-tab'
import { TroubleshootingTab } from '@/features/monitoring/troubleshooting-tab'
import { SystemLogsTab } from '@/features/monitoring/system-logs-tab'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'alerts', label: 'Alerts' },
  { value: 'jobs', label: 'Jobs' },
  { value: 'notifications', label: 'Notifications' },
  { value: 'activity', label: 'Activity' },
  { value: 'audit', label: 'Audit' },
  { value: 'troubleshooting', label: 'Troubleshooting' },
  { value: 'logs', label: 'Logs' },
] as const

export function MonitoringPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const tab = TABS.some((item) => item.value === requestedTab) ? requestedTab! : 'overview'

  function setTab(value: string) {
    const next = new URLSearchParams(searchParams)
    next.set('tab', value)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Monitoring"
        description="Live system health, alerts, jobs and event history."
      />
      <Tabs value={tab} onValueChange={setTab} className="gap-6">
        <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
          {TABS.map((item) => (
            <TabsTrigger key={item.value} value={item.value}>
              {item.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview"><MonitoringOverviewTab /></TabsContent>
        <TabsContent value="alerts"><AlertsTab /></TabsContent>
        <TabsContent value="jobs"><JobsTab /></TabsContent>
        <TabsContent value="notifications"><NotificationsTab /></TabsContent>
        <TabsContent value="activity"><TimelineTab /></TabsContent>
        <TabsContent value="audit"><AuditTab /></TabsContent>
        <TabsContent value="troubleshooting"><TroubleshootingTab /></TabsContent>
        <TabsContent value="logs"><SystemLogsTab /></TabsContent>
      </Tabs>
    </div>
  )
}
