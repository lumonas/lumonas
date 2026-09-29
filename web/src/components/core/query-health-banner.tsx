import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'

export function QueryHealthBanner() {
  const queryClient = useQueryClient()
  const [failedCount, setFailedCount] = useState(0)
  const [retrying, setRetrying] = useState(false)

  useEffect(() => {
    const cache = queryClient.getQueryCache()
    const update = () => {
      setFailedCount(
        cache.getAll().filter((query) => query.state.status === 'error' && query.getObserversCount() > 0).length,
      )
    }
    update()
    return cache.subscribe(update)
  }, [queryClient])

  if (failedCount === 0) return null

  return (
    <div className="fixed inset-x-4 top-16 z-50 md:left-64 md:right-8">
      <AlertBanner
        tone="warning"
        title={`${failedCount} system check${failedCount === 1 ? '' : 's'} unavailable`}
        action={
          <Button
            size="sm"
            variant="outline"
            disabled={retrying}
            onClick={async () => {
              setRetrying(true)
              try {
                await queryClient.invalidateQueries({ type: 'active' })
              } finally {
                setRetrying(false)
              }
            }}
          >
            <RefreshCw className={retrying ? 'animate-spin' : undefined} />
            {retrying ? 'Retrying…' : 'Retry'}
          </Button>
        }
      >
        Some panels may be incomplete. Existing data remains visible where available.
      </AlertBanner>
    </div>
  )
}
