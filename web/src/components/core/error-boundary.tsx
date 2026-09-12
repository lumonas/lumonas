import { Component, type ErrorInfo, type ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface ErrorBoundaryState {
  error: Error | null
}

export class ErrorBoundary extends Component<{ children: ReactNode }, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('UI crashed', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-dvh items-center justify-center bg-background px-4">
          <div className="w-full max-w-md rounded-xl border bg-card p-6 shadow-xs">
            <div className="flex items-center gap-2 text-sm font-medium text-critical">
              <TriangleAlert className="size-4" />
              Something went wrong
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              The interface hit an unexpected error while rendering. No settings were changed and
              the NAS is unaffected.
            </p>
            <pre className="mt-3 max-h-32 overflow-auto rounded-lg border bg-muted/30 p-3 font-mono text-xs text-muted-foreground">
              {this.state.error.message}
            </pre>
            <div className="mt-4 flex gap-2">
              <Button size="sm" onClick={() => window.location.reload()}>
                Reload interface
              </Button>
              <Button size="sm" variant="outline" onClick={() => (window.location.href = '/')}>
                Back to overview
              </Button>
            </div>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
