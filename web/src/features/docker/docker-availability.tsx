import { AlertBanner } from '@/components/core/alert-banner'
import type { DockerSummary } from '@/api/types'

export function DockerAvailabilityBanner({ summary }: { summary?: DockerSummary }) {
  if (!summary || summary.available) return null

  return (
    <AlertBanner tone="attention" title="Docker Engine is unavailable">
      Docker-backed apps, containers, images, and volumes cannot be refreshed until <span className="font-mono">docker.service</span> is running and the Docker socket is accessible.
    </AlertBanner>
  )
}
