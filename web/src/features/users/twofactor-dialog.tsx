import { useState, type FormEvent } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Copy, KeyRound } from 'lucide-react'
import { toast } from 'sonner'
import {
  useDisableTwoFactor,
  useEnableTwoFactor,
  useSetupTwoFactor,
  type TwoFactorSetup,
} from '@/api/queries'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { ManagementUser } from '@/api/types'

interface TwoFactorDialogProps {
  user: ManagementUser | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TwoFactorDialog({ user, open, onOpenChange }: TwoFactorDialogProps) {
  const setup = useSetupTwoFactor()
  const enable = useEnableTwoFactor()
  const disable = useDisableTwoFactor()
  const [enrolment, setEnrolment] = useState<TwoFactorSetup | null>(null)
  const [code, setCode] = useState('')

  if (!user) return null
  const busy = setup.isPending || enable.isPending || disable.isPending

  function startSetup() {
    setup.mutate(user!.id, {
      onSuccess: (value) => {
        setEnrolment(value)
        setCode('')
      },
    })
  }

  function confirmCode(event: FormEvent) {
    event.preventDefault()
    if (!user) return
    enable.mutate(
      { id: user.id, code: code.trim() },
      {
        onSuccess: () => {
          toast.success(`Two-factor authentication enabled — ${user.username}`)
          onOpenChange(false)
        },
      },
    )
  }

  function turnOff() {
    if (!user) return
    disable.mutate(user.id, {
      onSuccess: () => {
        toast.success(`Two-factor authentication disabled — ${user.username}`)
        onOpenChange(false)
      },
    })
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setEnrolment(null)
      setCode('')
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-lg">
        {user.twoFactor ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <KeyRound className="size-4 text-success" />
                Two-factor is enabled — {user.username}
              </DialogTitle>
              <DialogDescription>
                Sign-in requires a 6-digit code from the authenticator app. Recovery codes were
                shown during setup and work exactly once each.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
                Close
              </Button>
              <Button variant="destructive" onClick={turnOff} disabled={busy}>
                {disable.isPending ? 'Disabling…' : 'Disable two-factor'}
              </Button>
            </DialogFooter>
          </>
        ) : enrolment ? (
          <form onSubmit={confirmCode}>
            <DialogHeader>
              <DialogTitle>Scan with your authenticator</DialogTitle>
              <DialogDescription>
                Use Google Authenticator, Aegis, 1Password or any TOTP app, then enter the 6-digit
                code to finish.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col items-center gap-4 py-4">
              <div className="rounded-lg border bg-white p-3">
                <QRCodeSVG value={enrolment.otpauthUri} size={168} />
              </div>
              <button
                type="button"
                className="flex items-center gap-1.5 font-mono text-xs text-muted-foreground"
                onClick={() => {
                  void navigator.clipboard.writeText(enrolment.secret)
                  toast.success('Secret copied')
                }}
              >
                <Copy className="size-3" />
                {enrolment.secret}
              </button>
              <div className="w-full rounded-lg border bg-muted/40 p-3">
                <p className="text-xs font-medium text-muted-foreground">
                  Recovery codes — each works once. Store them now; they are not shown again.
                </p>
                <div className="mt-2 grid grid-cols-2 gap-1 font-mono text-xs">
                  {enrolment.recoveryCodes.map((recoveryCode) => (
                    <span key={recoveryCode}>{recoveryCode}</span>
                  ))}
                </div>
              </div>
              <div className="w-full">
                <Label htmlFor="totp-code">Verification code</Label>
                <Input
                  id="totp-code"
                  inputMode="numeric"
                  pattern="\d{6}"
                  maxLength={6}
                  value={code}
                  onChange={(event) => setCode(event.target.value.replace(/\D/g, ''))}
                  className="mt-2 text-center font-mono text-lg tracking-[0.4em]"
                  autoFocus
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setEnrolment(null)}
                disabled={busy}
              >
                Back
              </Button>
              <Button type="submit" disabled={code.length !== 6 || enable.isPending}>
                {enable.isPending ? 'Verifying…' : 'Enable two-factor'}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Enable two-factor — {user.username}</DialogTitle>
              <DialogDescription>
                Two-factor requires a recovery key on the appliance. Codes are generated by an
                authenticator app and requested at every sign-in.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
                Cancel
              </Button>
              <Button onClick={startSetup} disabled={busy}>
                {setup.isPending ? 'Generating…' : 'Begin setup'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
