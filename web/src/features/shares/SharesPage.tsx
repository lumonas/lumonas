import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, FolderOpen, Plus, Share2 } from 'lucide-react'
import { apiGet, apiPost } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

type Protocol = { name: string; settings?: Record<string, unknown> }
type Share = { id: string; name: string; path: string; description?: string; enabled: boolean; guest: boolean; protocols: Protocol[]; access: { principalName?: string; level: string }[] }
const protocolOptions = ['smb', 'nfs', 'sftp', 'ftp', 'ftps', 'rsync']

export function SharesPage() {
  const queryClient = useQueryClient()
  const shares = useQuery({ queryKey: ['admin', 'shares'], queryFn: () => apiGet<Share[]>('/shares') })
  const [step, setStep] = useState(1)
  const [name, setName] = useState('')
  const [path, setPath] = useState('/srv/pools/data')
  const [description, setDescription] = useState('')
  const [protocols, setProtocols] = useState<string[]>(['smb'])
  const [guest, setGuest] = useState(false)
  const [access, setAccess] = useState('')
  const [message, setMessage] = useState('')
  const createShare = useMutation({
    mutationFn: () => apiPost<Share>('/shares', {
      name, path, description, enabled: true, guest,
      protocols: protocols.map((protocol) => ({ name: protocol })),
      access: access.split(',').map((item) => item.trim()).filter(Boolean).map((item) => { const [principalName, level = 'read'] = item.split(':'); return { principalName, level } }),
    }),
    onSuccess: () => { setStep(1); setName(''); setDescription(''); setMessage('Share created and configuration activated.'); void queryClient.invalidateQueries({ queryKey: ['admin', 'shares'] }) },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to create share.'),
  })
  const toggleProtocol = (protocol: string) => setProtocols((current) => current.includes(protocol) ? current.filter((item) => item !== protocol) : [...current, protocol])

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Shares" description="Publish pool directories through validated SMB, NFS, SFTP, FTP, FTPS, or rsync policies." actions={<Button onClick={() => setStep(1)}><Plus />New share</Button>} />
      <div className="grid gap-4 xl:grid-cols-[1.45fr_1fr]">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Share2 className="size-4 text-primary" />Published shares</CardTitle><CardDescription>Every change is validated and activated atomically.</CardDescription></CardHeader>
          <CardContent>
            {shares.isLoading ? <p className="text-sm text-muted-foreground">Loading shares…</p> : shares.isError ? <p className="text-sm text-critical">Unable to load shares. Check the API connection.</p> : <Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Path</TableHead><TableHead>Protocols</TableHead><TableHead>Access</TableHead></TableRow></TableHeader><TableBody>{shares.data?.map((share) => <TableRow key={share.id}><TableCell className="font-medium">{share.name}<div className="text-xs text-muted-foreground">{share.guest ? 'Guest enabled' : 'Authenticated access'}</div></TableCell><TableCell className="font-mono text-xs">{share.path}</TableCell><TableCell><div className="flex flex-wrap gap-1">{share.protocols.map((protocol) => <Badge key={protocol.name} variant="secondary">{protocol.name}</Badge>)}</div></TableCell><TableCell>{share.access.length ? `${share.access.length} rule${share.access.length === 1 ? '' : 's'}` : 'No explicit rules'}</TableCell></TableRow>)}</TableBody></Table>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><FolderOpen className="size-4 text-primary" />Create share</CardTitle><CardDescription>Four steps keep path, protocol, access, and review concerns separate.</CardDescription></CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">{['Path', 'Protocols', 'Access', 'Review'].map((label, index) => <span key={label} className={index + 1 === step ? 'font-semibold text-primary' : ''}>{index + 1}. {label}</span>)}</div>
            {step === 1 ? <div className="space-y-3"><Label htmlFor="share-name">Share name</Label><Input id="share-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="documents" /><Label htmlFor="share-path">Directory path</Label><Input id="share-path" value={path} onChange={(event) => setPath(event.target.value)} placeholder="/srv/pools/data/documents" /><Label htmlFor="share-description">Description</Label><Input id="share-description" value={description} onChange={(event) => setDescription(event.target.value)} placeholder="Team documents" /></div> : null}
            {step === 2 ? <div className="space-y-2">{protocolOptions.map((protocol) => <label key={protocol} className="flex items-center justify-between rounded-md border px-3 py-2 text-sm uppercase"><span>{protocol}</span><Switch checked={protocols.includes(protocol)} onCheckedChange={() => toggleProtocol(protocol)} /></label>)}</div> : null}
            {step === 3 ? <div className="space-y-3"><label className="flex items-center justify-between rounded-md border px-3 py-2 text-sm"><span>Allow guest access</span><Switch checked={guest} onCheckedChange={setGuest} /></label><Label htmlFor="share-access">Access rules</Label><Input id="share-access" value={access} onChange={(event) => setAccess(event.target.value)} placeholder="family:write, backup:read" /><p className="text-xs text-muted-foreground">Use principal names and levels: none, read, or write.</p></div> : null}
            {step === 4 ? <div className="rounded-lg border bg-muted/30 p-3 text-sm"><p className="font-medium">{name || 'Unnamed share'}</p><p className="mt-1 font-mono text-xs text-muted-foreground">{path}</p><p className="mt-2">{protocols.join(', ') || 'No protocols selected'} · {guest ? 'guest enabled' : 'authenticated'}</p><p className="mt-1 text-muted-foreground">{access || 'No explicit access rules'}</p></div> : null}
            <div className="flex justify-between gap-2"><Button variant="outline" disabled={step === 1} onClick={() => setStep((current) => current - 1)}><ArrowLeft />Back</Button>{step < 4 ? <Button disabled={(step === 1 && (!name || !path)) || (step === 2 && protocols.length === 0)} onClick={() => setStep((current) => current + 1)}>Next<ArrowRight /></Button> : <Button disabled={createShare.isPending} onClick={() => createShare.mutate()}>Create share</Button>}</div>
          </CardContent>
        </Card>
      </div>
      {message ? <p className="text-sm text-muted-foreground" role="status">{message}</p> : null}
    </div>
  )
}
