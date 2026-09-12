import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { setupServer } from 'msw/node'
import { describe, expect, it, beforeAll, afterAll, afterEach } from 'vitest'
import { NetworkMonitoring } from '@/features/network/network-monitoring'
import { useMetricsStore } from '@/stores/metrics'

const server = setupServer()

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => {
  server.resetHandlers()
  useMetricsStore.setState({
    metrics: {
      cpuPercent: 0,
      load: [0, 0, 0],
      ramUsedBytes: 0,
      ramTotalBytes: 0,
      cpuTempC: 0,
      uptimeSeconds: 0,
      net: { interface: 'eth0', upMbps: 0, downMbps: 0 },
      disk: { readMbps: 0, writeMbps: 0 },
    },
    ifaceHistories: {},
  })
})

function renderWithQuery(ui: React.ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('NetworkMonitoring', () => {
  it('shows waiting message when no interfaces', async () => {
    renderWithQuery(<NetworkMonitoring />)
    await waitFor(() => {
      expect(screen.getByText(/Waiting for interface metrics/)).toBeInTheDocument()
    })
  })

  it('renders interface cards when data is present', async () => {
    useMetricsStore.setState({
      metrics: {
        cpuPercent: 0,
        load: [0, 0, 0],
        ramUsedBytes: 0,
        ramTotalBytes: 0,
        cpuTempC: 0,
        uptimeSeconds: 0,
        net: { interface: 'eth0', upMbps: 0, downMbps: 0 },
        netInterfaces: [
          { interface: 'eth0', upMbps: 10, downMbps: 50, errorsIn: 0, errorsOut: 0, droppedIn: 0, droppedOut: 0, up: true },
          { interface: 'eth1', upMbps: 0, downMbps: 0, errorsIn: 3, errorsOut: 1, droppedIn: 0, droppedOut: 0, up: true },
        ],
        disk: { readMbps: 0, writeMbps: 0 },
      },
    })
    renderWithQuery(<NetworkMonitoring />)
    await waitFor(() => {
      expect(screen.getByText('eth0')).toBeInTheDocument()
      expect(screen.getByText('eth1')).toBeInTheDocument()
    })
  })

  it('shows error counts for interfaces with errors', async () => {
    useMetricsStore.setState({
      metrics: {
        cpuPercent: 0,
        load: [0, 0, 0],
        ramUsedBytes: 0,
        ramTotalBytes: 0,
        cpuTempC: 0,
        uptimeSeconds: 0,
        net: { interface: 'eth0', upMbps: 0, downMbps: 0 },
        netInterfaces: [
          { interface: 'eth0', upMbps: 0, downMbps: 0, errorsIn: 5, errorsOut: 2, droppedIn: 1, droppedOut: 0, up: true },
        ],
        disk: { readMbps: 0, writeMbps: 0 },
      },
    })
    renderWithQuery(<NetworkMonitoring />)
    await waitFor(() => {
      expect(screen.getByText(/7 err/)).toBeInTheDocument()
      expect(screen.getByText(/1 drop/)).toBeInTheDocument()
    })
  })
})
