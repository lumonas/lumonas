import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { describe, expect, it, beforeAll, afterAll, afterEach } from 'vitest'
import { HealthExplanation } from '@/components/core/health-explanation'

const server = setupServer(
  http.get('/api/v1/health/components', () =>
    HttpResponse.json({
      status: 'healthy',
      score: 100,
      components: [
        { id: 'disks', label: 'Disk health', status: 'healthy', message: '2 disk(s) healthy', recommended: '' },
        { id: 'protection', label: 'SnapRAID protection', status: 'healthy', message: 'Parity configured', recommended: '' },
      ],
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

describe('HealthExplanation', () => {
  it('renders health score', async () => {
    renderWithQuery(<HealthExplanation />)
    await waitFor(() => {
      expect(screen.getByText('Health score')).toBeInTheDocument()
      expect(screen.getByText('100')).toBeInTheDocument()
    })
  })

  it('renders component labels', async () => {
    renderWithQuery(<HealthExplanation />)
    await waitFor(() => {
      expect(screen.getByText('Disk health')).toBeInTheDocument()
      expect(screen.getByText('SnapRAID protection')).toBeInTheDocument()
    })
  })
})
