import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { AlertHistoryCard } from '@/features/monitoring/alert-history-card'

const server = setupServer()

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

function renderWithQuery() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AlertHistoryCard />
    </QueryClientProvider>,
  )
}

describe('AlertHistoryCard', () => {
  it('renders resolved alerts with their lifecycle timestamps', async () => {
    const resolvedAt = new Date(Date.now() - 60_000).toISOString()
    server.use(
      http.get('/api/v1/alerts/history', () =>
        HttpResponse.json([
          {
            id: 'alert-resolved',
            severity: 'warning',
            title: 'Disk temperature recovered',
            description: 'The disk is back below the configured threshold.',
            resource: { type: 'disk', id: 'disk-1' },
            state: 'resolved',
            startedAt: new Date(Date.now() - 120_000).toISOString(),
            resolvedAt,
          },
        ]),
      ),
    )

    renderWithQuery()

    await waitFor(() => {
      expect(screen.getByText('Disk temperature recovered')).toBeInTheDocument()
      expect(screen.getByText(/resolved 1m ago/)).toBeInTheDocument()
      expect(screen.getByText('resolved')).toBeInTheDocument()
    })
  })

  it('shows an empty state when nothing has resolved', async () => {
    server.use(http.get('/api/v1/alerts/history', () => HttpResponse.json([])))
    renderWithQuery()

    await waitFor(() => {
      expect(screen.getByText('No resolved alerts yet.')).toBeInTheDocument()
    })
  })
})
