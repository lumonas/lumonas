import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { lazy, Suspense, useEffect } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
import { AuthGate } from '@/components/auth/AuthGate'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ErrorBoundary } from '@/components/core/error-boundary'
import { QueryHealthBanner } from '@/components/core/query-health-banner'
import { AppShell } from '@/components/layout/AppShell'
import { useOnboardingState } from '@/api/queries'
import { useUiStore } from '@/stores/ui'
import { ThemeProvider } from '@/theme/ThemeProvider'
import { useTheme } from '@/theme/use-theme'

const DashboardPage = lazy(() => import('@/features/dashboard/DashboardPage').then((module) => ({ default: module.DashboardPage })))
const BackupsPage = lazy(() => import('@/features/backups/BackupsPage').then((module) => ({ default: module.BackupsPage })))
const DockerPage = lazy(() => import('@/features/docker/DockerPage').then((module) => ({ default: module.DockerPage })))
const FilesPage = lazy(() => import('@/features/files/FilesPage').then((module) => ({ default: module.FilesPage })))
const MyFilesPage = lazy(() => import('@/features/files/MyFilesPage').then((module) => ({ default: module.MyFilesPage })))
const MonitoringPage = lazy(() => import('@/features/monitoring/MonitoringPage').then((module) => ({ default: module.MonitoringPage })))
const NetworkPage = lazy(() => import('@/features/network/NetworkPage').then((module) => ({ default: module.NetworkPage })))
const OnboardingPage = lazy(() => import('@/features/onboarding/OnboardingPage').then((module) => ({ default: module.OnboardingPage })))
const SharesPage = lazy(() => import('@/features/shares/SharesPage').then((module) => ({ default: module.SharesPage })))
const SettingsPage = lazy(() => import('@/features/settings/SettingsPage').then((module) => ({ default: module.SettingsPage })))
const StoragePage = lazy(() => import('@/features/storage/StoragePage').then((module) => ({ default: module.StoragePage })))
const UsersPage = lazy(() => import('@/features/users/UsersPage').then((module) => ({ default: module.UsersPage })))
const UpdatesPage = lazy(() => import('@/features/updates/UpdatesPage').then((module) => ({ default: module.UpdatesPage })))
const InstallPage = lazy(() => import('@/features/installer/InstallPage').then((module) => ({ default: module.InstallPage })))
const DependencyGraphPage = lazy(() => import('@/features/dependencies/DependencyGraphPage').then((module) => ({ default: module.DependencyGraphPage })))
const PublicFileRequestPage = lazy(() => import('@/features/files/PublicFileRequestPage').then((module) => ({ default: module.PublicFileRequestPage })))
const PublicFileSharePage = lazy(() => import('@/features/files/PublicFileSharePage').then((module) => ({ default: module.PublicFileSharePage })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 1,
      throwOnError: false,
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
  const isInstallerRoute = window.location.pathname === '/install'
  const isPublicRequestRoute = window.location.pathname.startsWith('/request/')
  const isPublicShareRoute = window.location.pathname.startsWith('/share/')
  return (
    <BrowserRouter>
      <Suspense fallback={<main role="status" aria-live="polite" className="mx-auto flex min-h-64 max-w-7xl items-center justify-center px-6 text-sm text-muted-foreground">Loading page…</main>}>
        {!onboarding?.completed && !isInstallerRoute && !isPublicRequestRoute && !isPublicShareRoute ? (
          <OnboardingPage />
        ) : (
          <Routes>
        <Route path="install" element={<InstallPage />} />
        <Route path="request/:token" element={<PublicFileRequestPage />} />
        <Route path="share/:token" element={<PublicFileSharePage />} />
        <Route path="my-files" element={<MyFilesPage />} />
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="storage" element={<StoragePage />} />
          <Route path="docker" element={<DockerPage />} />
          <Route path="monitoring" element={<MonitoringPage />} />
          <Route path="network" element={<NetworkPage />} />
          <Route path="dependencies" element={<DependencyGraphPage />} />
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
      </Suspense>
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
            <QueryHealthBanner />
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
