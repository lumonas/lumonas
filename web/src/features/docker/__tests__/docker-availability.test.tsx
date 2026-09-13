import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { DockerPage } from '@/features/docker/DockerPage'

const server = setupServer(
  http.get('/api/v1/docker/summary', () =>
    HttpResponse.json({ stacks: 0, appsRunning: 0, updatesAvailable: 0, available: false }),
  ),
  http.get('/api/v1/docker/apps', () => HttpResponse.json([])),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <DockerPage />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('Docker availability', () => {
  it('explains that empty Docker views are caused by an unavailable engine', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Docker Engine is unavailable')).toBeInTheDocument()
    })
    expect(screen.getByText(/docker\.service/)).toBeInTheDocument()
  })

  it('does not show an outage banner when the engine is available', async () => {
    server.use(
      http.get('/api/v1/docker/summary', () =>
        HttpResponse.json({ stacks: 1, appsRunning: 1, updatesAvailable: 0, available: true }),
      ),
    )
    renderPage()

    await waitFor(() => expect(screen.getByText('Apps')).toBeInTheDocument())
    expect(screen.queryByText('Docker Engine is unavailable')).not.toBeInTheDocument()
  })
})
