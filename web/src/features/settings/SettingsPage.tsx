import { useSearchParams } from 'react-router-dom'
import { History } from 'lucide-react'
import { PageHeader } from '@/components/core/page-header'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PowerTab } from '@/features/settings/power-tab'
import { RuntimeTab } from '@/features/settings/runtime-tab'
import { SecurityTab } from '@/features/settings/security-tab'
import { UpdatesTab } from '@/features/settings/updates-tab'
import { useOnboardingStore } from '@/stores/onboarding'

const TABS = [
  { value: 'updates', label: 'Updates' },
  { value: 'runtime', label: 'Runtime' },
  { value: 'power', label: 'Power & UPS' },
  { value: 'security', label: 'Security' },
] as const

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const resetOnboarding = useOnboardingStore((s) => s.reset)
  const tabParam = searchParams.get('tab')
  const tab = tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'updates'

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Settings"
        description="Updates, runtime behavior, power and security. Every change becomes a new config generation."
        actions={
          <Button variant="outline" size="sm" onClick={resetOnboarding}>
            <History />
            Re-run setup wizard
          </Button>
        }
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
        <TabsContent value="updates">
          <UpdatesTab />
        </TabsContent>
        <TabsContent value="runtime">
          <RuntimeTab />
        </TabsContent>
        <TabsContent value="power">
          <PowerTab />
        </TabsContent>
        <TabsContent value="security">
          <SecurityTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
