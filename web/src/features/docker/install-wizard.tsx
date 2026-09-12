import { useMemo, useState } from 'react'
import { CheckCircle2, Eye, EyeOff, Loader2 } from 'lucide-react'
import { useCreateStack } from '@/api/queries'
import { STORAGE_RESOURCES } from '@/api/resources'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type { CatalogApp } from '@/api/types'

const STEPS = ['Configuration', 'Review', 'Deploy'] as const

function randomSecret(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(15))
  return Array.from(bytes, (b) => b.toString(36).padStart(2, '0')).join('').slice(0, 20)
}

export function InstallWizard({
  app,
  onOpenChange,
}: {
  app: CatalogApp | null
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={app != null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogTitle className="sr-only">Install app</DialogTitle>
        {app ? (
          <WizardBody key={app.id} app={app} onClose={() => onOpenChange(false)} />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function WizardBody({ app, onClose }: { app: CatalogApp; onClose: () => void }) {
  const createStack = useCreateStack()
  const [step, setStep] = useState(0)
  const [name, setName] = useState(app.name.toLowerCase())
  const [values, setValues] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {}
    for (const field of app.form) {
      if (field.type === 'secret') initial[field.id] = randomSecret()
      else if (field.type !== 'timezone' && field.type !== 'storage_ref')
        initial[field.id] = field.defaultValue ?? ''
    }
    return initial
  })
  const [storage, setStorage] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {}
    for (const field of app.form) {
      if (field.type === 'storage_ref')
        initial[field.id] = field.defaultResource ?? STORAGE_RESOURCES[0].id
    }
    return initial
  })
  const [revealSecrets, setRevealSecrets] = useState(false)

  const secretFields = useMemo(
    () => new Set(app.form.filter((f) => f.type === 'secret').map((f) => f.id)),
    [app],
  )

  const valid =
    name.trim().length > 0 &&
    app.form.every((field) => {
      if (!field.required) return true
      if (field.type === 'storage_ref') return storage[field.id] != null
      if (field.type === 'timezone') return true
      return (values[field.id] ?? '').trim().length > 0
    })

  const portField = app.form.find((f) => f.type === 'port')

  function install() {
    createStack.mutate(
      {
        catalogId: app.id,
        name: name.trim(),
        env: values,
        storageMap: app.form
          .filter((f) => f.type === 'storage_ref')
          .map((f) => ({
            fieldId: f.id,
            containerPath: f.containerPath ?? `/${f.id.toLowerCase()}`,
            resourceId: storage[f.id],
          })),
      },
      { onSuccess: () => setStep(2) },
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="text-lg font-semibold leading-none">Install {app.name}</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {app.tagline} — you can change everything later without editing YAML.
        </p>
      </div>

      <ol className="flex items-center gap-2" aria-label="Steps">
        {STEPS.map((label, index) => (
          <li key={label} className="flex flex-1 items-center gap-2">
            <span
              className={cn(
                'flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold',
                index < step && 'border-success bg-success/15 text-success',
                index === step && 'border-primary bg-primary/15 text-primary',
                index > step && 'border-border text-muted-foreground',
              )}
            >
              {index < step ? <CheckCircle2 className="size-3.5" /> : index + 1}
            </span>
            <span
              className={cn(
                'text-xs',
                index === step ? 'font-medium text-foreground' : 'text-muted-foreground',
              )}
            >
              {label}
            </span>
            {index < STEPS.length - 1 && <span className="h-px flex-1 bg-border" />}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <div className="flex max-h-[46vh] flex-col gap-4 overflow-y-auto pr-1">
          <div className="grid gap-2">
            <Label htmlFor="install-name">Stack name</Label>
            <Input
              id="install-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="font-mono"
            />
          </div>
          {app.form.map((field) => {
            if (field.type === 'timezone') {
              return (
                <div key={field.id} className="grid gap-2">
                  <Label>{field.label}</Label>
                  <p className="rounded-md border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
                    Europe/Warsaw — filled from your server settings
                  </p>
                </div>
              )
            }
            if (field.type === 'storage_ref') {
              return (
                <div key={field.id} className="grid gap-2">
                  <Label htmlFor={`field-${field.id}`}>
                    {field.label}
                    {field.containerPath ? (
                      <span className="ml-2 font-mono text-xs text-muted-foreground">
                        → {field.containerPath}
                      </span>
                    ) : null}
                  </Label>
                  <Select
                    value={storage[field.id]}
                    onValueChange={(next) =>
                      setStorage((current) => ({ ...current, [field.id]: next }))
                    }
                  >
                    <SelectTrigger id={`field-${field.id}`}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {STORAGE_RESOURCES.map((resource) => (
                        <SelectItem key={resource.id} value={resource.id}>
                          {resource.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {field.description ? (
                    <p className="text-xs text-muted-foreground">{field.description}</p>
                  ) : null}
                </div>
              )
            }
            if (field.type === 'secret') {
              return (
                <div key={field.id} className="grid gap-2">
                  <Label htmlFor={`field-${field.id}`}>{field.label}</Label>
                  <div className="relative">
                    <Input
                      id={`field-${field.id}`}
                      type={revealSecrets ? 'text' : 'password'}
                      value={values[field.id] ?? ''}
                      onChange={(e) =>
                        setValues((current) => ({ ...current, [field.id]: e.target.value }))
                      }
                      className="pr-9 font-mono"
                    />
                    <button
                      type="button"
                      onClick={() => setRevealSecrets((v) => !v)}
                      className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                      aria-label={revealSecrets ? 'Hide secret' : 'Reveal secret'}
                    >
                      {revealSecrets ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                    </button>
                  </div>
                  <p className="text-xs text-muted-foreground">{field.description}</p>
                </div>
              )
            }
            return (
              <div key={field.id} className="grid gap-2">
                <Label htmlFor={`field-${field.id}`}>
                  {field.label}
                  {field.required ? <span className="ml-1 text-critical">*</span> : null}
                </Label>
                <Input
                  id={`field-${field.id}`}
                  type={field.type === 'port' ? 'number' : 'text'}
                  value={values[field.id] ?? ''}
                  onChange={(e) =>
                    setValues((current) => ({ ...current, [field.id]: e.target.value }))
                  }
                />
                {field.description ? (
                  <p className="text-xs text-muted-foreground">{field.description}</p>
                ) : null}
              </div>
            )
          })}
        </div>
      )}

      {step === 1 && (
        <div className="flex max-h-[46vh] flex-col gap-4 overflow-y-auto pr-1">
          <div className="rounded-lg border">
            <p className="border-b bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Storage mappings
            </p>
            <ul className="divide-y">
              <li className="flex items-center gap-2 px-3 py-2 text-sm">
                <span className="font-mono text-xs text-muted-foreground">/config</span>
                <span aria-hidden className="text-muted-foreground">→</span>
                <span>Apps SSD / {name}</span>
              </li>
              {app.form
                .filter((f) => f.type === 'storage_ref')
                .map((field) => (
                  <li key={field.id} className="flex items-center gap-2 px-3 py-2 text-sm">
                    <span className="font-mono text-xs text-muted-foreground">
                      {field.containerPath}
                    </span>
                    <span aria-hidden className="text-muted-foreground">→</span>
                    <span>{STORAGE_RESOURCES.find((r) => r.id === storage[field.id])?.label}</span>
                  </li>
                ))}
            </ul>
          </div>
          <div className="rounded-lg border">
            <p className="border-b bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              Environment
            </p>
            <ul className="divide-y">
              <li className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                <span className="font-mono text-xs">TZ</span>
                <span className="text-muted-foreground">Europe/Warsaw</span>
              </li>
              {Object.entries(values).map(([key, value]) => (
                <li
                  key={key}
                  className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
                >
                  <span className="font-mono text-xs">{key}</span>
                  <span
                    className={cn(
                      'font-mono text-xs',
                      secretFields.has(key) && 'text-muted-foreground',
                    )}
                  >
                    {secretFields.has(key) ? '••••••••••' : value}
                  </span>
                </li>
              ))}
            </ul>
          </div>
          {portField && (
            <p className="text-sm text-muted-foreground">
              Web UI will be available on port{' '}
              <span className="tnum font-medium text-foreground">
                {values[portField.id] || portField.defaultValue}
              </span>
              .
            </p>
          )}
          <p className="rounded-lg bg-muted/40 px-3 py-2.5 text-xs text-muted-foreground">
            Deployment runs as a background job: pull images, create containers, health check.
            Configuration is snapshotted first.
          </p>
        </div>
      )}

      {step === 2 && (
        <div className="flex flex-col items-center gap-3 py-8 text-center">
          {createStack.isError ? (
            <>
              <CheckCircle2 className="size-10 text-critical" />
              <p className="text-sm font-medium">Installation failed to start</p>
              <p className="text-sm text-muted-foreground">
                A stack with this name may already exist.
              </p>
            </>
          ) : (
            <>
              <CheckCircle2 className="size-10 text-success" />
              <p className="text-sm font-medium">Installing {app.name}</p>
              <p className="max-w-sm text-sm text-muted-foreground">
                Deployment is running as a background job. You can close this dialog and track
                progress from the jobs menu in the top bar.
              </p>
            </>
          )}
        </div>
      )}

      {step < 2 && (
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          {step === 1 ? (
            <Button variant="ghost" onClick={() => setStep(0)}>
              Back
            </Button>
          ) : null}
          {step === 0 ? (
            <Button disabled={!valid} onClick={() => setStep(1)}>
              Continue
            </Button>
          ) : (
            <Button onClick={install} disabled={createStack.isPending}>
              {createStack.isPending && <Loader2 className="animate-spin" />}
              Install app
            </Button>
          )}
        </div>
      )}
      {step === 2 && (
        <div className="flex justify-end">
          <Button onClick={onClose}>Done</Button>
        </div>
      )}
    </div>
  )
}
