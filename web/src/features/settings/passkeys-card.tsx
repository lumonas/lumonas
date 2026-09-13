import { useCallback, useEffect, useState } from 'react'
import { Fingerprint, Plus, Trash2 } from 'lucide-react'
import { apiDelete, apiGet } from '@/api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { passkeysSupported, registerPasskey } from '@/lib/webauthn'
import type { Passkey } from '@/lib/webauthn'

export function PasskeysCard() {
  const [passkeys, setPasskeys] = useState<Passkey[]>([])
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const supported = passkeysSupported()

  const load = useCallback(async () => {
    if (!supported) return
    try {
      setPasskeys(await apiGet<Passkey[]>('/users/self/passkeys'))
      setError(null)
    } catch {
      setError('Passkeys could not be loaded')
    }
  }, [supported])

  useEffect(() => {
    let active = true
    if (!supported) return () => undefined
    void apiGet<Passkey[]>('/users/self/passkeys')
      .then((value) => {
        if (active) {
          setPasskeys(value)
          setError(null)
        }
      })
      .catch(() => {
        if (active) setError('Passkeys could not be loaded')
      })
    return () => {
      active = false
    }
  }, [supported])

  if (!supported) return null

  async function addPasskey() {
    setBusy(true)
    try {
      await registerPasskey('admin', `Passkey ${new Date().toLocaleDateString()}`)
      await load()
    } catch {
      setError('Passkey registration was cancelled or failed')
    } finally {
      setBusy(false)
    }
  }

  async function removePasskey(id: string) {
    try {
      await apiDelete(`/users/self/passkeys/${encodeURIComponent(id)}`)
      await load()
    } catch {
      setError('Passkey could not be removed')
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Fingerprint className="size-4" /> Passkeys
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {passkeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No passkeys registered. A passkey replaces password + second factor with your device's
            biometrics or security key.
          </p>
        ) : (
          <div className="flex flex-col divide-y rounded-lg border">
            {passkeys.map((passkey) => (
              <div key={passkey.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                <div>
                  <p className="text-sm">{passkey.name}</p>
                  <p className="text-xs text-muted-foreground">
                    Added {new Date(passkey.createdAt).toLocaleDateString()}
                  </p>
                </div>
                <Button variant="ghost" size="icon-sm" onClick={() => void removePasskey(passkey.id)}>
                  <Trash2 className="text-destructive" />
                </Button>
              </div>
            ))}
          </div>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div>
          <Button variant="outline" size="sm" onClick={() => void addPasskey()} disabled={busy}>
            <Plus />
            {busy ? 'Waiting for device…' : 'Add passkey'}
          </Button>
        </div>
        <Badge variant="outline" className="w-fit text-muted-foreground">
          Works over HTTPS or localhost
        </Badge>
      </CardContent>
    </Card>
  )
}
