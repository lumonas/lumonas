import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { TooltipProvider } from '@/components/ui/tooltip'
import { AppShell } from '@/components/layout/AppShell'
import { DashboardPage } from '@/features/dashboard/DashboardPage'
import { PlaceholderPage } from '@/features/placeholder/PlaceholderPage'
import { StoragePage } from '@/features/storage/StoragePage'
import { NetworkPage } from '@/features/network/NetworkPage'
import { SharesPage } from '@/features/shares/SharesPage'
import { UsersPage } from '@/features/users/UsersPage'
import { ThemeProvider } from '@/theme/ThemeProvider'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 1,
    },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <ThemeProvider>
          <BrowserRouter>
            <Routes>
              <Route element={<AppShell />}>
                <Route index element={<DashboardPage />} />
                <Route path="storage" element={<StoragePage />} />
                <Route path="network" element={<NetworkPage />} />
                <Route path="shares" element={<SharesPage />} />
                <Route path="users" element={<UsersPage />} />
                <Route path="*" element={<PlaceholderPage />} />
              </Route>
            </Routes>
          </BrowserRouter>
        </ThemeProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}
