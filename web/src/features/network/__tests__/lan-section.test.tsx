import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { LanHostsSection } from '@/features/network/lan-section'

const server = setupServer()

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

function renderWithQuery() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <LanHostsSection />
    </QueryClientProvider>,
  )
}

describe('LanHostsSection', () => {
  it('renders known hosts and marks stale observations', async () => {
    server.use(
      http.get('/api/v1/network/lan/hosts', () =>
        HttpResponse.json([
          {
            mac: 'aa:bb:cc:dd:ee:10',
            interface: 'eth0',
            ip: '192.168.1.10',
            hostname: 'media-player',
            firstSeen: new Date(Date.now() - 86_400_000).toISOString(),
            lastSeen: new Date(Date.now() - 60_000).toISOString(),
          },
          {
            mac: 'aa:bb:cc:dd:ee:11',
            interface: 'eth0',
            ip: '192.168.1.11',
            firstSeen: new Date(Date.now() - 10 * 86_400_000).toISOString(),
            lastSeen: new Date(Date.now() - 8 * 86_400_000).toISOString(),
          },
        ]),
      ),
    )

    renderWithQuery()

    await waitFor(() => {
      expect(screen.getByText('media-player')).toBeInTheDocument()
      expect(screen.getByText('offline?')).toBeInTheDocument()
      expect(screen.getAllByRole('button', { name: 'Wake' })).toHaveLength(2)
    })
  })

  it('scans, wakes, and renames a known host through typed API calls', async () => {
    let scans = 0
    let wakeBody: unknown
    let renameBody: unknown
    server.use(
      http.get('/api/v1/network/lan/hosts', () =>
        HttpResponse.json([{
          mac: 'aa:bb:cc:dd:ee:10',
          interface: 'eth0',
          ip: '192.168.1.10',
          firstSeen: new Date().toISOString(),
          lastSeen: new Date().toISOString(),
        }]),
      ),
      http.post('/api/v1/network/lan/scan', () => {
        scans += 1
        return HttpResponse.json([])
      }),
      http.post('/api/v1/network/lan/hosts/wake', async ({ request }) => {
        wakeBody = await request.json()
        return HttpResponse.json({ status: 'wake packet sent' })
      }),
      http.post('/api/v1/network/lan/hosts/rename', async ({ request }) => {
        renameBody = await request.json()
        return HttpResponse.json({ status: 'renamed' })
      }),
    )

    renderWithQuery()
    await waitFor(() => expect(screen.getByText('192.168.1.10')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Scan now' }))
    fireEvent.click(screen.getByRole('button', { name: 'Wake' }))
    fireEvent.click(screen.getByRole('button', { name: 'Rename device' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Hostname' }), { target: { value: 'nas-backup' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(scans).toBe(1)
      expect(wakeBody).toEqual({ mac: 'aa:bb:cc:dd:ee:10', interface: 'eth0' })
      expect(renameBody).toEqual({ mac: 'aa:bb:cc:dd:ee:10', interface: 'eth0', hostname: 'nas-backup' })
    })
  })
})
