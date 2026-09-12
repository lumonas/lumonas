import { lazy, Suspense, useState } from 'react'
import { Archive, ArrowUpCircle, Eye, EyeOff, Play, RotateCw, Square } from 'lucide-react'
import { useDockerContainers, useDockerStack, useStackAction } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { HealthBadge } from '@/components/core/health-badge'
import { LogStream } from '@/components/core/log-stream'
import { Metric } from '@/components/core/metric'
import { OperationReview } from '@/components/core/operation-review'
import { ResourceDrawer } from '@/components/core/resource-drawer'
import { EmptyState } from '@/components/core/empty-state'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Skeleton } from '@/components/ui/skeleton'
import { StateChip } from '@/features/docker/docker-meta'
import { RISK_LABELS } from '@/features/docker/state-meta'
import { UpdateWizard } from '@/features/docker/update-wizard'
import { formatBytes, timeAgo } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { DockerStack } from '@/api/types'

const ComposeEditor = lazy(() =>
  import('@/features/docker/compose-editor').then((m) => ({ default: m.ComposeEditor })),
)

export function StackDrawer({
  stackId,
  onOpenChange,
}: {
  stackId: string | null
  onOpenChange: (open: boolean) => void
}) {
  const { data: stack } = useDockerStack(stackId)
  const { data: containers } = useDockerContainers()
  const stackAction = useStackAction()

  const [updateOpen, setUpdateOpen] = useState(false)
  const [revealedSecrets, setRevealedSecrets] = useState<Set<string>>(new Set())

  if (!stack) {
    return null
  }

  const stackContainers = (containers ?? []).filter((c) => c.stackId === stack.id)
  const running = stack.state === 'running'

  function toggleReveal(name: string) {
    setRevealedSecrets((current) => {
      const next = new Set(current)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <ResourceDrawer
      open={stackId != null}
      onOpenChange={onOpenChange}
      title={
        <span className="flex items-center gap-2.5">
          <span className="font-mono">{stack.name}</span>
          <StateChip state={stack.state} kind="stack" />
        </span>
      }
      description={
        <span className="flex items-center gap-2">
          <HealthBadge state={stack.status} />
          <span className="text-xs">{stack.category}</span>
        </span>
      }
      footer={
        <div className="flex flex-wrap items-center gap-2">
          {running ? (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={() => stackAction.mutate({ id: stack.id, action: 'restart' })}
              >
                <RotateCw />
                Restart
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => stackAction.mutate({ id: stack.id, action: 'stop' })}
              >
                <Square />
                Stop
              </Button>
            </>
          ) : (
            <Button
              size="sm"
              variant="outline"
              onClick={() => stackAction.mutate({ id: stack.id, action: 'start' })}
            >
              <Play />
              Start
            </Button>
          )}
          {stack.updateAvailable && (
            <Button size="sm" onClick={() => setUpdateOpen(true)}>
              <ArrowUpCircle />
              Update to {stack.updateAvailable.latest}
            </Button>
          )}
          <span className="ml-auto text-xs text-muted-foreground">
            Deployed {timeAgo(stack.lastDeploy)}
          </span>
        </div>
      }
    >
      <Tabs defaultValue="overview">
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="storage">Storage</TabsTrigger>
          <TabsTrigger value="environment">Environment</TabsTrigger>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="compose">Compose</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          {stack.risks.length > 0 && (
            <div className="flex flex-col gap-2">
              {stack.risks.map((risk) => (
                <AlertBanner key={risk} tone="warning" title={RISK_LABELS[risk]}>
                  This construct is preserved — LumoNAS only warns about it.
                </AlertBanner>
              ))}
            </div>
          )}
          {stack.updateAvailable && (
            <AlertBanner
              tone="info"
              title={`Update available — ${stack.updateAvailable.current} → ${stack.updateAvailable.latest}`}
              action={
                <Button size="sm" variant="outline" onClick={() => setUpdateOpen(true)}>
                  Review
                </Button>
              }
            >
              Updates are never installed automatically.
            </AlertBanner>
          )}
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <Metric label="CPU" value={`${stack.cpuPercent}%`} />
            <Metric label="Memory" value={formatBytes(stack.ramUsedBytes)} />
            <Metric label="Restarts" value={stack.restarts} />
            <Metric label="Containers" value={stackContainers.length} />
          </div>
          {stack.ports.length > 0 && (
            <div>
              <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                Ports
              </p>
              <ul className="flex flex-col divide-y rounded-lg border">
                {stack.ports.map((port) => (
                  <li
                    key={`${port.host}-${port.container}`}
                    className="flex items-center justify-between px-3 py-2 text-sm"
                  >
                    <span className="tnum">
                      :{port.host} → {port.container}
                    </span>
                    <span className="text-xs">
                      <a
                        href={`http://${window.location.hostname}:${port.host}`}
                        target="_blank"
                        rel="noreferrer"
                        className="text-primary hover:underline"
                      >
                        {port.label ?? 'Open'} ↗
                      </a>
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div className="rounded-xl border p-4">
            <div className="flex items-center gap-2 text-sm font-medium">
              <Archive className="size-4 text-muted-foreground" />
              Backup
            </div>
            <div className="mt-3 grid gap-3 sm:grid-cols-1">
              <div className="grid content-start gap-2">
                <span className="text-xs text-muted-foreground">Appdata</span>
                <span className="tnum text-sm">{formatBytes(stack.backup.appdataSizeBytes)}</span>
                <span className="text-xs text-muted-foreground">
                  {stack.backup.lastBackupAt
                    ? `Last backup ${timeAgo(stack.backup.lastBackupAt)}`
                    : 'Never backed up'}
                </span>
              </div>
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              Stack updates run an encrypted configuration backup beforehand; per-app appdata
              backup is planned.
            </p>
          </div>
        </TabsContent>

        <TabsContent value="storage">
          {stack.storage.length === 0 ? (
            <EmptyState
              title="No storage mappings"
              description="This stack was imported — LumoNAS could not infer any bind mounts."
              className="border-0"
            />
          ) : (
            <div className="flex flex-col gap-3 rounded-lg border">
              <p className="border-b bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                Bind mounts
              </p>
              <ul className="flex flex-col divide-y border-0">
                {stack.storage.map((mapping) => (
                  <li
                    key={mapping.containerPath}
                    className="flex flex-wrap items-center gap-2 px-3 py-2.5 text-sm"
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      {mapping.containerPath}
                    </span>
                    <span aria-hidden className="text-muted-foreground">→</span>
                    <span>{mapping.resourceLabel}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            Paths are stored as storage references — moving internal mount points later will not
            break this stack.
          </p>
        </TabsContent>

        <TabsContent value="environment">
          {stack.env.length === 0 ? (
            <EmptyState
              title="No variables"
              description="This stack does not declare tracked environment variables."
              className="border-0"
            />
          ) : (
            <div className="overflow-hidden rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/40">
                    <th className="px-3 py-2 text-left text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      Name
                    </th>
                    <th className="px-3 py-2 text-left text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      Value
                    </th>
                    <th className="px-3 py-2 text-left text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      Scope
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {stack.env.map((variable) => {
                    const isSecret = variable.scope === 'secret'
                    const revealed = revealedSecrets.has(variable.name)
                    return (
                      <tr key={variable.name} className="border-b last:border-b-0">
                        <td className="px-3 py-2 font-mono text-xs">{variable.name}</td>
                        <td className="px-3 py-2">
                          <span className="flex items-center gap-2">
                            <span
                              className={cn(
                                'font-mono text-xs',
                                isSecret && !revealed && 'text-muted-foreground',
                              )}
                            >
                              {isSecret && !revealed ? '••••••••••••' : variable.value}
                            </span>
                            {isSecret && (
                              <button
                                type="button"
                                onClick={() => toggleReveal(variable.name)}
                                className="text-muted-foreground hover:text-foreground"
                                aria-label={revealed ? 'Hide reference' : 'Reveal reference'}
                              >
                                {revealed ? (
                                  <EyeOff className="size-3.5" />
                                ) : (
                                  <Eye className="size-3.5" />
                                )}
                              </button>
                            )}
                          </span>
                        </td>
                        <td className="px-3 py-2">
                          <span className="text-xs text-muted-foreground">
                            {variable.scope === 'builtin' ? (
                              'LumoNAS built-in'
                            ) : (
                              <span className="capitalize">{variable.scope}</span>
                            )}
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            Secrets live in the LumoNAS secret store — they are never written to plain .env files or
            exports.
          </p>
        </TabsContent>

        <TabsContent value="logs">
          <LogStream containers={stackContainers.map((c) => c.name)} />
        </TabsContent>

        <TabsContent value="compose">
          <ComposeTab key={stack.id} stack={stack} />
        </TabsContent>
      </Tabs>

      <UpdateWizard stack={stack} open={updateOpen} onOpenChange={setUpdateOpen} />
    </ResourceDrawer>
  )
}

function ComposeTab({ stack }: { stack: DockerStack }) {
  const stackAction = useStackAction()
  const { data: containers } = useDockerContainers()
  const [compose, setCompose] = useState(stack.composeYaml)
  const [lastServer, setLastServer] = useState(stack.composeYaml)
  const [validated, setValidated] = useState(false)
  const [reviewOpen, setReviewOpen] = useState(false)

  if (stack.composeYaml !== lastServer) {
    setLastServer(stack.composeYaml)
    if (compose === lastServer) setCompose(stack.composeYaml)
  }

  const dirty = compose !== stack.composeYaml
  const containerCount =
    (containers ?? []).filter((c) => c.stackId === stack.id).length || 1

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => {
            setValidated(true)
            setTimeout(() => setValidated(false), 4000)
          }}
        >
          Validate
        </Button>
        {validated && (
          <span className="text-xs text-success">
            Compose is valid — no port or storage conflicts.
          </span>
        )}
        {dirty && <span className="text-xs text-attention">Unsaved changes</span>}
        <Button
          size="sm"
          className="ml-auto"
          disabled={!dirty || stackAction.isPending}
          onClick={() => setReviewOpen(true)}
        >
          Deploy changes
        </Button>
      </div>
      <Suspense fallback={<Skeleton className="h-[340px] w-full" />}>
        <ComposeEditor value={compose} onChange={setCompose} />
      </Suspense>
      <p className="text-xs text-muted-foreground">
        Changes made here are preserved. Unknown options and comments are kept as-is.
      </p>

      <OperationReview
        open={reviewOpen}
        onOpenChange={setReviewOpen}
        title={`Deploy ${stack.name}`}
        description="Review what this deployment will do before applying."
        steps={[
          'Snapshot current configuration',
          `Recreate ${containerCount} container${containerCount === 1 ? '' : 's'}`,
          'Health check after start',
        ]}
        warnings={stack.risks.map((risk) => ({ label: RISK_LABELS[risk] }))}
        diff={{ before: stack.composeYaml, after: compose }}
        confirmLabel="Deploy"
        loading={stackAction.isPending}
        onConfirm={() => {
          stackAction.mutate(
            { id: stack.id, action: 'deploy', body: { composeYaml: compose } },
            { onSuccess: () => setReviewOpen(false) },
          )
        }}
      />
    </>
  )
}
