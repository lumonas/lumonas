import { useState } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { describe, expect, it, beforeAll, afterAll, afterEach, vi } from 'vitest'
import { DiskOperationsDialog, diskBranchPath, type DiskAction } from '@/features/storage/disk-operations-dialog'
import type { Disk } from '@/api/types'

const PLAN = {
  operationId: 'op-123',
  action: 'filesystem.format',
  target: { diskId: 'wwn:test', wwn: 'test', serial: 'SER-1', model: 'Test Disk', sizeBytes: 1000 },
  requestedState: { filesystem: 'ext4' },
  dependencySnapshot: [],
  configGeneration: 1,
  expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
  planHash: 'abcdef1234567890abcdef1234567890',
  status: 'planned',
}

const planHandler = vi.fn()
const confirmHandler = vi.fn()

const server = setupServer(
  http.get('/api/v1/storage/safety', () => HttpResponse.json({ state: 'unlocked', unlockedUntil: null })),
  http.post('/api/v1/storage/operations/plan', async ({ request }) => {
    planHandler(await request.json())
    return HttpResponse.json(PLAN, { status: 201 })
  }),
  http.post('/api/v1/storage/operations/:id/confirm', async ({ request }) => {
    confirmHandler(await request.json())
    return HttpResponse.json({ ok: true })
  }),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => {
  server.resetHandlers()
  planHandler.mockClear()
  confirmHandler.mockClear()
})

const disk: Disk = {
  id: 'wwn:test',
  name: 'sda',
  model: 'Test Disk',
  serial: 'SER-1',
  wwn: 'test',
  sizeBytes: 1000,
  role: 'data',
  rotational: true,
  interface: 'sata',
  health: 'healthy',
  temperatureC: 32,
  filesystem: 'ext4',
  mounted: false,
  lastSeen: new Date().toISOString(),
  smart: {
    overall: 'healthy',
    reallocatedSectors: 0,
    pendingSectors: 0,
    uncorrectableSectors: 0,
    crcErrors: 0,
    powerOnHours: 1000,
  },
}

function DialogHarness({ action }: { action: DiskAction }) {
  const [open, setOpen] = useState(true)
  return (
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <DiskOperationsDialog disk={disk} action={open ? action : null} onOpenChange={setOpen} />
      <span data-testid="closed">{open ? 'open' : 'closed'}</span>
    </QueryClientProvider>
  )
}

describe('diskBranchPath', () => {
  it('sanitizes stable identities into canonical branch paths', () => {
    expect(diskBranchPath('wwn:test')).toBe('/srv/disks/wwn_test')
    expect(diskBranchPath('serial:ABC-123')).toBe('/srv/disks/serial_ABC-123')
  })
})

describe('DiskOperationsDialog', () => {
  it('plans a format operation with the chosen filesystem and confirms it', async () => {
    const user = userEvent.setup()
    render(<DialogHarness action="format" />)
    await user.click(screen.getByRole('button', { name: 'Plan operation' }))
    await waitFor(() => {
      expect(planHandler).toHaveBeenCalledWith({
        action: 'filesystem.format',
        diskId: 'wwn:test',
        requestedState: { filesystem: 'ext4' },
      })
    })
    await waitFor(() => {
      expect(screen.getByText(/Plan hash:/)).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: /Confirm format/i }))
    await waitFor(() => {
      expect(confirmHandler).toHaveBeenCalledWith({
        planHash: PLAN.planHash,
        reauthenticated: true,
        storageSafetyUnlocked: true,
      })
    })
    await waitFor(() => {
      expect(screen.getByTestId('closed')).toHaveTextContent('closed')
    })
  })

  it('sends a canonical mount path with an optional label for format & mount', async () => {
    const user = userEvent.setup()
    render(<DialogHarness action="format-mount" />)
    await user.type(screen.getByLabelText(/Volume label/i), 'media')
    await user.click(screen.getByRole('button', { name: 'Plan operation' }))
    await waitFor(() => {
      expect(planHandler).toHaveBeenCalledWith({
        action: 'filesystem.create',
        diskId: 'wwn:test',
        requestedState: { filesystem: 'ext4', mountPath: '/srv/disks/wwn_test', label: 'media' },
      })
    })
  })

  it('surfaces plan rejection errors', async () => {
    server.use(
      http.post('/api/v1/storage/operations/plan', () =>
        HttpResponse.json({ error: 'target is currently mounted' }, { status: 409 }),
      ),
    )
    const user = userEvent.setup()
    render(<DialogHarness action="format" />)
    await user.click(screen.getByRole('button', { name: 'Plan operation' }))
    await waitFor(() => {
      expect(screen.getByText('Plan rejected')).toBeInTheDocument()
    })
    expect(screen.getByText(/target is currently mounted/)).toBeInTheDocument()
  })
})
