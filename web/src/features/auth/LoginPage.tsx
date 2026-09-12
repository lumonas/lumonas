import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useAuthStore } from '@/stores/auth'
import { useServer } from '@/api/queries'
import { Logo } from '@/components/layout/Logo'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function LoginPage() {
  const login = useAuthStore((s) => s.login)
  const { data: server } = useServer()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [failed, setFailed] = useState(false)
  const [pending, setPending] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (password.length < 4 || pending) return
    setPending(true)
    setFailed(false)
    const ok = await login(username.trim(), password)
    setPending(false)
    if (!ok) setFailed(true)
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-2 text-center">
          <Logo className="size-10" />
          <h1 className="text-xl font-semibold tracking-tight">LumoNAS</h1>
          <p className="text-sm text-muted-foreground">
            {server ? `${server.name} · ${server.ip}` : 'Sign in to manage your NAS'}
          </p>
        </div>
        <Card>
          <CardContent className="p-5">
            <form onSubmit={submit} className="flex flex-col gap-4">
              {failed && (
                <AlertBanner tone="critical" title="Sign-in failed">
                  Check the username and password, then try again. Repeated failures are recorded
                  in the audit log.
                </AlertBanner>
              )}
              <div className="grid gap-2">
                <Label htmlFor="login-username">Username</Label>
                <Input
                  id="login-username"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="login-password">Password</Label>
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="current-password"
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
              <Button type="submit" disabled={pending || password.length < 4}>
                {pending && <Loader2 className="animate-spin" />}
                Sign in
              </Button>
            </form>
          </CardContent>
        </Card>
        <p className="mt-4 text-center text-xs text-muted-foreground">
          Demo build — sign in as <span className="font-mono">admin</span> with any password of 4+
          characters.
        </p>
      </div>
    </div>
  )
}
