import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { RuntimeTab } from '@/features/settings/runtime-tab'
import { settings } from '@/mocks/settings'

const patchHandler = vi.fn()
const server = setupServer(
  http.get('/api/v1/settings', () => HttpResponse.json(settings)),
  http.patch('/api/v1/settings', async ({ request }) => {
    patchHandler(await request.json())
    return HttpResponse.json(settings)
  }),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => {
  server.resetHandlers()
  patchHandler.mockClear()
})

function renderRuntime() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RuntimeTab />
    </QueryClientProvider>,
  )
}

describe('RuntimeTab', () => {
  it('shows live tmpfs state and sends a typed toggle patch', async () => {
    const user = userEvent.setup()
    renderRuntime()
    await screen.findByText('RAM transcode cache')
    expect(screen.getByText('/var/tmp/lumonas-transcode')).toBeInTheDocument()

    await user.click(screen.getByRole('switch', { name: 'Toggle RAM transcode cache' }))
    await waitFor(() => expect(patchHandler).toHaveBeenCalled())
    expect(patchHandler).toHaveBeenCalledWith({
      section: 'runtime',
      patch: {
        tmpfs: expect.objectContaining({ enabled: false, sizeBytes: 1_073_741_824 }),
      },
    })
  })
})
