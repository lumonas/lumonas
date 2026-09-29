import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Lock, ShieldCheck, UserRoundCog } from 'lucide-react'
import { apiPost } from '@/api/client'
import { useAPITokens, useRevokeOtherSessions, useRevokeSession, useSettings, useTLSCertificateStatus, useUpdateSettings, useUsers } from '@/api/queries'
import type { Job } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { SSHKeysDialog } from '@/features/settings/ssh-keys-dialog'
import { PasskeysCard } from '@/features/settings/passkeys-card'
import { APITokensCard } from '@/features/settings/api-tokens-card'
import { timeAgo } from '@/lib/format'
import { useCurrentTime } from '@/hooks/useCurrentTime'
import { toast } from 'sonner'

export function SecurityTab() {
  const client = useQueryClient()
  const [certificateFile, setCertificateFile] = useState<File | null>(null)
  const [privateKeyFile, setPrivateKeyFile] = useState<File | null>(null)
  const [confirmCertificateRotation, setConfirmCertificateRotation] = useState(false)
  const [acmeDomain, setAcmeDomain] = useState('')
  const [acmeEmail, setAcmeEmail] = useState('')
  const [agreeToACMETerms, setAgreeToACMETerms] = useState(false)
  const [allowACMEPort, setAllowACMEPort] = useState(false)
  const [confirmDisableACME, setConfirmDisableACME] = useState(false)
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  const revokeSession = useRevokeSession()
  const revokeOtherSessions = useRevokeOtherSessions()
  const { data: users } = useUsers()
  const { data: tokens } = useAPITokens()
  const certificate = useTLSCertificateStatus()
  const importCertificate = useMutation({
    mutationFn: async () => {
      if (!certificateFile || !privateKeyFile) throw new Error('Choose both certificate and private key files')
      return apiPost<Job>('/security/certificate/import', {
        certificate: await certificateFile.text(),
        privateKey: await privateKeyFile.text(),
      })
    },
    onSuccess: async (job) => {
      toast.success(`Certificate installation queued as ${job.id}`)
      setCertificateFile(null)
      setPrivateKeyFile(null)
      setConfirmCertificateRotation(false)
      await Promise.all([
        client.invalidateQueries({ queryKey: ['security', 'certificate'] }),
        client.invalidateQueries({ queryKey: ['jobs'] }),
      ])
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not install certificate'),
  })
  const configureACME = useMutation({
    mutationFn: () => apiPost<Job>('/security/certificate/acme', { domain: acmeDomain.trim(), email: acmeEmail.trim(), agreeToTerms: agreeToACMETerms, allowPort80: allowACMEPort }),
    onSuccess: async (job) => {
      toast.success(`Let's Encrypt setup queued as ${job.id}`)
      setAgreeToACMETerms(false)
      setAllowACMEPort(false)
      await Promise.all([client.invalidateQueries({ queryKey: ['security', 'certificate'] }), client.invalidateQueries({ queryKey: ['jobs'] })])
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not configure automatic renewal'),
  })
  const disableACME = useMutation({
    mutationFn: () => apiPost<{ autoRenewalConfigured: boolean }>('/security/certificate/acme/disable'),
    onSuccess: async () => {
      toast.success('ACME renewal and its local firewall rule were disabled; the active certificate remains installed')
      setConfirmDisableACME(false)
      await client.invalidateQueries({ queryKey: ['security', 'certificate'] })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not disable automatic renewal'),
  })
  const now = useCurrentTime()
  if (!settings) return null
  const { security } = settings

  return (
    <div className="flex flex-col gap-4">
      <PasskeysCard />
      <APITokensCard />
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><UserRoundCog className="size-4" />Access review</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3">
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div className="rounded-md border px-3 py-2"><p className="text-lg font-semibold">{users?.management.filter((user) => user.enabled).length ?? '—'}</p><p className="text-xs text-muted-foreground">enabled users</p></div>
            <div className="rounded-md border px-3 py-2"><p className="text-lg font-semibold">{users?.management.filter((user) => user.enabled && user.role === 'owner').length ?? '—'}</p><p className="text-xs text-muted-foreground">owners</p></div>
            <div className="rounded-md border px-3 py-2"><p className="text-lg font-semibold">{tokens?.length ?? '—'}</p><p className="text-xs text-muted-foreground">API tokens</p></div>
            <div className="rounded-md border px-3 py-2"><p className="text-lg font-semibold">{tokens?.filter((token) => !token.lastUsedAt || now - Date.parse(token.lastUsedAt) > 90 * 86_400_000).length ?? '—'}</p><p className="text-xs text-muted-foreground">unused 90+ days</p></div>
          </div>
          {(users?.management ?? []).filter((user) => user.enabled && user.role === 'owner').map((user) => <div key={user.id} className="flex items-center justify-between border-b pb-2 text-sm"><span>{user.fullName || user.username}</span><Badge variant="warning">{user.role}</Badge></div>)}
          {(tokens ?? []).filter((token) => !token.lastUsedAt || now - Date.parse(token.lastUsedAt) > 90 * 86_400_000).map((token) => <div key={token.id} className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-warning/30 px-3 py-2 text-xs"><span>{token.name} · {token.lastUsedAt ? `last used ${timeAgo(token.lastUsedAt)}` : 'never used'}</span><span className="font-mono text-muted-foreground">{token.scopes.join(', ')}</span></div>)}
          <p className="text-xs text-muted-foreground">Review administrator membership and revoke API tokens that are no longer in use from the sections above.</p>
        </CardContent>
      </Card>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between pb-3"><CardTitle className="text-sm font-medium text-muted-foreground">TLS certificate</CardTitle><Button size="sm" variant="outline" onClick={() => void certificate.refetch()} disabled={certificate.isFetching}>Refresh</Button></CardHeader>
          <CardContent className="space-y-2">
            {certificate.isLoading ? <p className="text-sm text-muted-foreground">Checking certificate…</p> : certificate.isError ? <p role="alert" className="text-sm text-destructive">Certificate status could not be checked.</p> : <>
              <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-sm font-medium">{certificate.data?.detail}</p><Badge variant={certificate.data?.state === 'valid' ? 'success' : certificate.data?.state === 'expiring' ? 'warning' : certificate.data?.state === 'disabled' ? 'secondary' : 'destructive'}>{certificate.data?.state}</Badge></div>
              {certificate.data?.configured && certificate.data.state !== 'missing' && certificate.data.state !== 'invalid' ? <div className="space-y-1 text-xs text-muted-foreground"><p>Subject: {certificate.data.subject || 'not provided'}</p><p>Issuer: {certificate.data.issuer || 'not provided'}</p><p>Expires: {certificate.data.notAfter ? new Date(certificate.data.notAfter).toLocaleString() : 'unknown'}{certificate.data.daysRemaining != null ? ` · ${certificate.data.daysRemaining} days remaining` : ''}</p>{certificate.data.dnsNames?.length ? <p className="break-all">Names: {certificate.data.dnsNames.join(', ')}</p> : null}</div> : null}
              <p className="text-[11px] text-muted-foreground">Private key material is only sent to the local daemon for installation and is never returned by the status API.</p>
            </>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">Replace certificate</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">Import a PEM certificate chain and its matching private key. LumoNAS validates the pair, installs it with restricted permissions, then restarts the web listener. Keep a browser tab open until the install job completes.</p>
            {certificate.data?.autoRenewalConfigured || certificate.data?.acmeFirewallConfigured ? <p role="status" className="rounded-md border border-warning/40 bg-warning/5 p-3 text-xs">Let's Encrypt or its HTTP challenge firewall rule is still configured. Disable ACME below before importing a manually managed certificate.</p> : <>
            <label className="block text-xs text-muted-foreground">Certificate chain (.pem, .crt)
              <input key={certificateFile ? 'certificate-selected' : 'certificate-empty'} aria-label="TLS certificate PEM file" type="file" accept=".pem,.crt,.cer,application/x-pem-file" onChange={(event) => setCertificateFile(event.target.files?.[0] ?? null)} className="mt-1 block w-full text-xs file:mr-3 file:rounded-md file:border file:bg-background file:px-3 file:py-1.5 file:text-xs" />
            </label>
            <label className="block text-xs text-muted-foreground">Private key (.pem, .key)
              <input key={privateKeyFile ? 'key-selected' : 'key-empty'} aria-label="TLS private key PEM file" type="file" accept=".pem,.key,application/x-pem-file" onChange={(event) => setPrivateKeyFile(event.target.files?.[0] ?? null)} className="mt-1 block w-full text-xs file:mr-3 file:rounded-md file:border file:bg-background file:px-3 file:py-1.5 file:text-xs" />
            </label>
            <label className="flex items-start gap-2 text-xs text-muted-foreground">
              <input aria-label="Confirm certificate replacement" type="checkbox" checked={confirmCertificateRotation} onChange={(event) => setConfirmCertificateRotation(event.target.checked)} className="mt-0.5" />
              <span>I understand this replaces the active certificate and restarts the web listener.</span>
            </label>
            <Button size="sm" onClick={() => importCertificate.mutate()} disabled={!certificateFile || !privateKeyFile || !confirmCertificateRotation || importCertificate.isPending}>
              {importCertificate.isPending ? 'Queueing…' : 'Validate and replace certificate'}
            </Button>
            </>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">Let's Encrypt automatic renewal</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {certificate.data?.autoRenewalConfigured ? <>
              <p className="text-xs text-muted-foreground">Certificate renewal is managed by Certbot. HTTP validation requires public DNS to resolve to this NAS and inbound TCP port 80 to reach it. The renewal timer will keep the active certificate updated.</p>
              <label className="flex items-start gap-2 text-xs text-muted-foreground"><input aria-label="Confirm disabling automatic renewal" type="checkbox" checked={confirmDisableACME} onChange={(event) => setConfirmDisableACME(event.target.checked)} className="mt-0.5" /><span>Stop Let's Encrypt renewal. The current certificate will stay installed, but will expire unless replaced.</span></label>
              <Button size="sm" variant="outline" disabled={!confirmDisableACME || disableACME.isPending} onClick={() => disableACME.mutate()}>{disableACME.isPending ? 'Disabling…' : 'Disable automatic renewal'}</Button>
            </> : certificate.data?.acmeFirewallConfigured ? <>
              <p className="text-xs text-warning">Automatic renewal is not configured, but the ACME TCP port 80 firewall rule remains open.</p>
              <label className="flex items-start gap-2 text-xs text-muted-foreground"><input aria-label="Confirm removing ACME firewall rule" type="checkbox" checked={confirmDisableACME} onChange={(event) => setConfirmDisableACME(event.target.checked)} className="mt-0.5" /><span>Remove the LumoNAS TCP port 80 firewall rule.</span></label>
              <Button size="sm" variant="outline" disabled={!confirmDisableACME || disableACME.isPending} onClick={() => disableACME.mutate()}>{disableACME.isPending ? 'Removing…' : 'Close ACME firewall rule'}</Button>
            </> : <>
              <p className="text-xs text-muted-foreground">Request a publicly trusted certificate with HTTP-01 validation. Point the domain at this NAS, forward inbound TCP port 80, and allow that port in the local firewall before starting. The request runs as a background job and renewal is scheduled automatically.</p>
              <label className="block text-xs text-muted-foreground">Public DNS name<input aria-label="ACME DNS name" autoComplete="url" value={acmeDomain} onChange={(event) => setAcmeDomain(event.target.value)} placeholder="nas.example.com" className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" /></label>
              <label className="block text-xs text-muted-foreground">Email for renewal notices<input aria-label="ACME contact email" type="email" autoComplete="email" value={acmeEmail} onChange={(event) => setAcmeEmail(event.target.value)} placeholder="admin@example.com" className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" /></label>
              <label className="flex items-start gap-2 text-xs text-muted-foreground"><input aria-label="Agree to ACME terms" type="checkbox" checked={agreeToACMETerms} onChange={(event) => setAgreeToACMETerms(event.target.checked)} className="mt-0.5" /><span>I agree to the Let's Encrypt terms of service and authorize a public certificate request.</span></label>
              <label className="flex items-start gap-2 text-xs text-muted-foreground"><input aria-label="Allow ACME TCP port 80" type="checkbox" checked={allowACMEPort} onChange={(event) => setAllowACMEPort(event.target.checked)} className="mt-0.5" /><span>Allow inbound TCP port 80 in the LumoNAS firewall for certificate validation and scheduled renewal. Router port forwarding is still required.</span></label>
              <Button size="sm" onClick={() => configureACME.mutate()} disabled={!acmeDomain.trim() || !acmeEmail.trim() || !agreeToACMETerms || !allowACMEPort || configureACME.isPending}>{configureACME.isPending ? 'Queueing…' : 'Configure automatic certificate renewal'}</Button>
            </>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <ShieldCheck className="size-4" />
              HTTPS
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm">Web listener certificate</p>
                <p className="text-xs text-muted-foreground">
                  TLS is configured by the server service. Certificate files are loaded when the
                  web listener starts.
                </p>
              </div>
              <Badge variant={certificate.data?.configured ? 'success' : 'secondary'}>
                {certificate.isLoading ? 'Checking…' : certificate.data?.configured ? 'Configured' : 'Disabled'}
              </Badge>
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div>
                <p className="text-sm">Automatic certificate renewal</p>
                <p className="text-xs text-muted-foreground">
                  ACME renewal is not configured on this server. Use the certificate status above
                  to monitor the active certificate.
                </p>
              </div>
              <Badge variant="secondary">Not configured</Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <Lock className="size-4" />
              SSH
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm">Root login</p>
                <p className="text-xs text-muted-foreground">Disabled and locked (recommended).</p>
              </div>
              <Badge variant="success">Locked</Badge>
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div>
                <p className="text-sm">Password authentication</p>
                <p className="text-xs text-muted-foreground">
                  Key-only is enforced while at least one admin key exists.
                </p>
              </div>
              <Switch
                checked={security.ssh.passwordAuth}
                onCheckedChange={(next) =>
                  updateSettings.mutate({
                    section: 'security',
                    patch: { ssh: { ...security.ssh, passwordAuth: next } },
                  })
                }
                aria-label="Toggle SSH password authentication"
              />
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div className="flex items-center gap-2 text-sm">
                <KeyRound className="size-4 text-muted-foreground" />
                Authorized keys
              </div>
              <div className="flex items-center gap-2">
                <Badge variant="secondary">{security.ssh.keyCount} key</Badge>
                <SSHKeysDialog />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex-row items-center justify-between pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Active sessions
          </CardTitle>
          <Button size="sm" variant="outline" disabled={revokeOtherSessions.isPending || security.sessions.filter((session) => !session.current).length === 0} onClick={() => { if (window.confirm('Sign out all other sessions? This device will stay signed in.')) revokeOtherSessions.mutate() }}>Revoke all other sessions</Button>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y rounded-lg border">
            {security.sessions.map((session) => (
              <li
                key={session.id}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5"
              >
                <div className="min-w-0">
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <span className="max-w-[55vw] truncate" title={session.device}>{session.device}</span>
                    {session.current && <Badge variant="secondary">this device</Badge>}
                  </p>
                  <p className="tnum text-xs text-muted-foreground">
                    {session.ip} · {session.scope} · active {timeAgo(session.lastActiveAt)}
                    {session.expiresAt ? ` · expires ${new Date(session.expiresAt).toLocaleString()}` : ''}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 text-xs"
                  disabled={session.current || revokeSession.isPending}
                  onClick={() => revokeSession.mutate(session.id)}
                >
                  Revoke
                </Button>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-muted-foreground">
            Revoking a session signs that device out immediately. Failed sign-in attempts appear in
            the audit log and can trigger alerts.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
