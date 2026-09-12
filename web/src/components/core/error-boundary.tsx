import { Component, type ErrorInfo, type ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface ErrorBoundaryState {
  error: Error | null
  info: ErrorInfo | null
}

export class ErrorBoundary extends Component<{ children: ReactNode }, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null, info: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error, info: null }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('UI crashed', error, info.componentStack)
    this.setState({ info })
  }

  render() {
    if (this.state.error) {
      const { error, info } = this.state
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
              {error.message}
            </pre>
            {import.meta.env.DEV && (error.stack || info?.componentStack) ? (
              <details className="mt-2">
                <summary className="cursor-pointer select-none text-xs font-medium text-muted-foreground">
                  Dev details: stack trace
                </summary>
                <pre className="mt-2 max-h-64 overflow-auto rounded-lg border bg-muted/30 p-3 font-mono text-xs text-muted-foreground">
                  {error.stack}
                  {info?.componentStack ? `\n\nComponent stack:${info.componentStack}` : ''}
                </pre>
              </details>
            ) : null}
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
