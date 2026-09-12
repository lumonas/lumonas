import { PageHeader } from '@/components/core/page-header'
import {
  DockerCard,
  HealthCard,
  NeedsAttentionCard,
  ProtectionCard,
  RecentActivityCard,
  StorageSummaryCard,
  SystemCard,
} from '@/features/dashboard/dashboard-cards'

export function DashboardPage() {
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Overview" description="Everything important about your NAS at a glance." />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <div className="lg:col-span-8">
          <HealthCard />
        </div>
        <div className="lg:col-span-4">
          <NeedsAttentionCard />
        </div>
        <div className="lg:col-span-4">
          <SystemCard />
        </div>
        <div className="lg:col-span-4">
          <StorageSummaryCard />
        </div>
        <div className="lg:col-span-4">
          <ProtectionCard />
        </div>
        <div className="lg:col-span-4">
          <DockerCard />
        </div>
        <div className="lg:col-span-8">
          <RecentActivityCard />
        </div>
      </div>
    </div>
  )
}
