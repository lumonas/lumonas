import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { AuthGate } from '@/components/auth/AuthGate'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ErrorBoundary } from '@/components/core/error-boundary'
import { AppShell } from '@/components/layout/AppShell'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { BackupsPage } from '@/features/backups/BackupsPage'
import { DockerPage } from '@/features/docker/DockerPage'
import { FilesPage } from '@/features/files/FilesPage'
import { MonitoringPage } from '@/features/monitoring/MonitoringPage'
import { NetworkPage } from '@/features/network/NetworkPage'
import { OnboardingPage } from '@/features/onboarding/OnboardingPage'
import { SharesPage } from '@/features/shares/SharesPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { StoragePage } from '@/features/storage/StoragePage'
import { UsersPage } from '@/features/users/UsersPage'
import { UpdatesPage } from '@/features/updates/UpdatesPage'
import { useOnboardingState } from '@/api/queries'
import { useUiStore } from '@/stores/ui'
import { ThemeProvider, useTheme } from '@/theme/ThemeProvider'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 1,
      throwOnError: true,
    },
  },
})

function AppToaster() {
  const { resolvedTheme } = useTheme()
  return (
    <Toaster
      theme={resolvedTheme}
      position="bottom-right"
      toastOptions={{
        style: {
          fontFamily: 'var(--font-sans)',
        },
      }}
    />
  )
}

function DensityEffect() {
  const density = useUiStore((s) => s.density)
  useEffect(() => {
    document.documentElement.dataset.density = density
  }, [density])
  return null
}

function RoutedApp() {
  const { data: onboarding } = useOnboardingState()
  return (
    <BrowserRouter>
      {!onboarding?.completed ? (
        <OnboardingPage />
      ) : (
        <Routes>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="storage" element={<StoragePage />} />
          <Route path="docker" element={<DockerPage />} />
          <Route path="monitoring" element={<MonitoringPage />} />
          <Route path="network" element={<NetworkPage />} />
          <Route path="shares" element={<SharesPage />} />
          <Route path="files" element={<FilesPage />} />
          <Route path="backups" element={<BackupsPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="updates" element={<UpdatesPage />} />
          <Route path="users" element={<UsersPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
      )}
    </BrowserRouter>
  )
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          <ErrorBoundary>
            <DensityEffect />
            <AuthGate>
              <RoutedApp />
            </AuthGate>
            <AppToaster />
          </ErrorBoundary>
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}
