import { useEffect, useState, type FormEvent, type ReactNode } from 'react'

type AuthStatus = { required: boolean; configured: boolean; authenticated: boolean }

export function AuthGate({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  async function load() {
    const response = await fetch('/api/v1/auth/status')
    if (!response.ok) throw new Error('Authentication status unavailable')
    setStatus((await response.json()) as AuthStatus)
  }

  useEffect(() => {
    let active = true
    void fetch('/api/v1/auth/status')
      .then(async (response) => {
        if (!response.ok) throw new Error('Authentication status unavailable')
        return (await response.json()) as AuthStatus
      })
      .then((value) => { if (active) setStatus(value) })
      .catch((reason: Error) => { if (active) setError(reason.message) })
    return () => { active = false }
  }, [])

  async function login(event: FormEvent) {
    event.preventDefault()
    setError('')
    const response = await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) })
    if (!response.ok) { setError('Invalid credentials'); return }
    setPassword('')
    await load()
  }

  if (!status && !error) return <div className="flex h-dvh items-center justify-center bg-background text-muted-foreground">Connecting to LumoNAS…</div>
  if (error && !status) return <div className="flex h-dvh items-center justify-center bg-background p-6 text-sm text-destructive">{error}</div>
  if (!status || !status.required || status.authenticated) return <>{children}</>
  if (!status.configured) return <main className="flex h-dvh items-center justify-center bg-background p-6"><div className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-xl"><p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">LumoNAS setup</p><h1 className="mt-2 text-2xl font-semibold text-foreground">Create the administrator account</h1><p className="mt-3 text-sm leading-6 text-muted-foreground">Set LUMONAS_ADMIN_PASSWORD in the daemon environment, restart lumonasd, and reload this page. Passwords are hashed before they are stored.</p></div></main>
  return (
    <main className="flex h-dvh items-center justify-center bg-background p-6">
      <form onSubmit={login} className="w-full max-w-sm space-y-5 rounded-xl border border-border bg-card p-6 shadow-xl">
        <div><p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">LumoNAS</p><h1 className="mt-2 text-2xl font-semibold text-foreground">Sign in</h1><p className="mt-1 text-sm text-muted-foreground">Use the local administrator account.</p></div>
        <label className="block text-sm text-muted-foreground">Username<input value={username} onChange={(event) => setUsername(event.target.value)} className="mt-2 w-full rounded-md border border-border bg-background px-3 py-2 text-foreground" autoComplete="username" /></label>
        <label className="block text-sm text-muted-foreground">Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} className="mt-2 w-full rounded-md border border-border bg-background px-3 py-2 text-foreground" autoComplete="current-password" /></label>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <button type="submit" className="w-full rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground">Sign in</button>
      </form>
    </main>
  )
}
