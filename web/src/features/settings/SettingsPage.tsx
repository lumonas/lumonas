import { useSearchParams } from 'react-router-dom'
import { LifeBuoy } from 'lucide-react'
import { apiDownload } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PowerTab } from '@/features/settings/power-tab'
import { RuntimeTab } from '@/features/settings/runtime-tab'
import { SecurityTab } from '@/features/settings/security-tab'
import { UpdatesTab } from '@/features/settings/updates-tab'
import { toast } from 'sonner'

const TABS = [
  { value: 'updates', label: 'Updates' },
  { value: 'runtime', label: 'Runtime' },
  { value: 'power', label: 'Power & UPS' },
  { value: 'security', label: 'Security' },
] as const

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab = tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'updates'

  async function downloadSupportBundle() {
    try {
      const blob = await apiDownload('/diagnostics/support-bundle')
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `lumonas-support-${new Date().toISOString().slice(0, 10)}.zip`
      anchor.click()
      URL.revokeObjectURL(url)
      toast.success('Support bundle downloaded')
    } catch {
      toast.error('Support bundle could not be created')
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Settings"
        description="Updates, runtime behavior, power and security. Every change becomes a new config generation."
        actions={
          <Button variant="outline" size="sm" onClick={() => void downloadSupportBundle()}>
            <LifeBuoy />
            Support bundle
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
