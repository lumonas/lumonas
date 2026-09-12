import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { describe, expect, it, beforeAll, afterAll, afterEach } from 'vitest'
import { WireGuardSection } from '@/features/network/wireguard-section'

const server = setupServer(
  http.get('/api/v1/network/wireguard/status', () =>
    HttpResponse.json({
      interface: 'wg0',
      ip: '10.0.0.1',
      listenPort: 51820,
      peers: 2,
      connected: true,
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

describe('WireGuardSection', () => {
  it('renders WireGuard title', async () => {
    renderWithQuery(<WireGuardSection />)
    await waitFor(() => {
      expect(screen.getByText('WireGuard')).toBeInTheDocument()
    })
  })

  it('displays status fields', async () => {
    renderWithQuery(<WireGuardSection />)
    await waitFor(() => {
      expect(screen.getByText('wg0')).toBeInTheDocument()
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      expect(screen.getByText('51820')).toBeInTheDocument()
    })
  })

  it('shows peer count', async () => {
    renderWithQuery(<WireGuardSection />)
    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument()
    })
  })

  it('shows configure button', async () => {
    renderWithQuery(<WireGuardSection />)
    await waitFor(() => {
      expect(screen.getByText('Configure')).toBeInTheDocument()
    })
  })
})
