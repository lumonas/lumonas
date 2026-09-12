import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { describe, expect, it, beforeAll, afterAll, afterEach } from 'vitest'
import { TailscaleSection } from '@/features/network/tailscale-section'

const server = setupServer(
  http.get('/api/v1/network/tailscale/status', () =>
    HttpResponse.json({
      installed: true,
      running: true,
      connected: true,
      backendState: 'Running',
      version: '1.60.0',
      tailscaleIp4: '100.64.0.1',
      tailscaleIp6: 'fd7a:115c:a1e0::1',
      hostName: 'mynas',
      exitNode: '',
      exitNodeAllow: false,
      subnetRoutes: ['192.168.1.0/24'],
    }),
  ),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

function renderWithQuery(ui: React.ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('TailscaleSection', () => {
  it('renders Tailscale title', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('Tailscale')).toBeInTheDocument()
    })
  })

  it('displays hostname', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('mynas')).toBeInTheDocument()
    })
  })

  it('displays IPv4 address', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('100.64.0.1')).toBeInTheDocument()
    })
  })

  it('displays version', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('1.60.0')).toBeInTheDocument()
    })
  })

  it('shows subnet routes', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('192.168.1.0/24')).toBeInTheDocument()
    })
  })

  it('shows disconnect button when connected', async () => {
    renderWithQuery(<TailscaleSection />)
    await waitFor(() => {
      expect(screen.getByText('Disconnect')).toBeInTheDocument()
    })
  })
})
