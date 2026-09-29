import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { AuditTab } from '@/features/monitoring/audit-tab'

const server = setupServer(
  http.get('/api/v1/audit', () =>
    HttpResponse.json({ entries: [
      {
        id: 'audit-1',
        timestamp: '2026-01-01T00:00:00Z',
        actor: 'operator',
        action: 'docker.stack.action',
        outcome: 'committed',
        correlationId: 'corr-1',
        operationId: 'op-1',
        planHash: 'plan-1',
        generation: 12,
        resourceType: 'stack',
        resourceId: 'media',
      },
    ], hasMore: false }),
  ),
  http.get('/api/v1/audit/retention', () => HttpResponse.json({ retentionDays: 365 })),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

describe('AuditTab', () => {
  it('renders typed operation tracing fields', async () => {
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <AuditTab />
      </QueryClientProvider>,
    )

    await waitFor(() => expect(screen.getByText('docker.stack.action')).toBeInTheDocument())
    expect(screen.getByText('op: op-1')).toBeInTheDocument()
    expect(screen.getByText('plan: plan-1')).toBeInTheDocument()
    expect(screen.getByText('req: corr-1')).toBeInTheDocument()
    expect(screen.getByText('generation: 12')).toBeInTheDocument()
  })
})
