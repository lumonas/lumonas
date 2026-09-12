import { useEffect } from 'react'
import { Outlet } from 'react-router-dom'
import { CommandPalette } from '@/components/layout/CommandPalette'
import { MobileNav } from '@/components/layout/MobileNav'
import { Sidebar } from '@/components/layout/Sidebar'
import { StatusBar } from '@/components/layout/StatusBar'
import { TopBar } from '@/components/layout/TopBar'
import { useEventStream } from '@/hooks/useEventStream'
import { useUiStore } from '@/stores/ui'

export function AppShell() {
  useEventStream()
  const togglePalette = useUiStore((s) => s.togglePalette)

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        togglePalette()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [togglePalette])

  return (
    <div className="flex h-dvh overflow-hidden">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col md:pl-60">
        <TopBar />
        <main className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1440px] px-4 pt-6 pb-24 md:px-8 md:pt-8 md:pb-10">
            <Outlet />
          </div>
        </main>
        <StatusBar className="hidden md:flex" />
      </div>
      <MobileNav />
      <CommandPalette />
    </div>
  )
}
