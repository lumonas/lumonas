import { useEffect, useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { CheckCircle2, CloudUpload } from 'lucide-react'
import { apiGet } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '@/lib/format'

type PublicRequest = { shareName: string; expiresAt: string; maxFiles: number; remainingFiles: number; maxBytes: number; remainingBytes: number }

export function PublicFileRequestPage() {
  const { token = '' } = useParams()
  const [request, setRequest] = useState<PublicRequest | null>(null)
  const [error, setError] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [sending, setSending] = useState(false)
  const [uploaded, setUploaded] = useState<string[]>([])

  useEffect(() => {
    let active = true
    void apiGet<PublicRequest>(`/public/file-requests/${encodeURIComponent(token)}`).then((value) => { if (active) setRequest(value) }).catch((reason: Error) => { if (active) setError(reason.message || 'This upload link is unavailable.') })
    return () => { active = false }
    return () => { active = false }
  }, [token])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (files.length === 0 || !request) return
    setSending(true); setError('')
    try {
      let current = request
      for (let index = 0; index < files.length; index += 1) {
        const file = files[index]!
        if (current.remainingFiles <= 0) throw new Error('This upload request has reached its file limit.')
        if (file.size > current.remainingBytes) throw new Error(`${file.name} is larger than the remaining ${formatBytes(current.remainingBytes)} allowance.`)
        const form = new FormData(); form.append('file', file)
        const response = await fetch(`/api/v1/public/file-requests/${encodeURIComponent(token)}/upload`, { method: 'POST', body: form })
        const body = await response.json() as { error?: string; name?: string; remainingFiles?: number; remainingBytes?: number }
        if (!response.ok) throw new Error(body.error || `Upload failed for ${file.name}`)
        current = { ...current, remainingFiles: body.remainingFiles ?? Math.max(0, current.remainingFiles - 1), remainingBytes: body.remainingBytes ?? current.remainingBytes - file.size }
        setRequest(current)
        setUploaded((previous) => [...previous, body.name || file.name])
        setFiles((previous) => previous.slice(1))
      }
      setFiles([])
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Upload failed') }
    finally { setSending(false) }
  }

  if (!request && !error) return <main className="flex min-h-dvh items-center justify-center p-6"><p role="status">Checking upload link…</p></main>
  return <main className="flex min-h-dvh items-center justify-center bg-muted/30 p-4"><Card className="w-full max-w-lg">
    <CardHeader><div className="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary"><CloudUpload className="size-5" /></div><CardTitle>{request ? 'Send files to LumoNAS' : 'Upload link unavailable'}</CardTitle><p className="text-sm text-muted-foreground">{request ? `Upload files for ${request.shareName}. You cannot browse or download anything in this folder.` : error}</p></CardHeader>
    {request ? <CardContent className="space-y-4"><div className="rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">{request.remainingFiles} file{request.remainingFiles === 1 ? '' : 's'} remaining · {formatBytes(request.remainingBytes)} remaining · link expires {new Date(request.expiresAt).toLocaleString()}</div>
      {uploaded.length > 0 ? <div role="status" className="rounded-md border border-success/40 bg-success/5 p-3 text-sm"><p className="flex items-center gap-2 font-medium"><CheckCircle2 className="size-4 text-success" />{uploaded.length} file{uploaded.length === 1 ? '' : 's'} uploaded successfully.</p><ul className="mt-2 list-inside list-disc text-xs text-muted-foreground">{uploaded.map((name, index) => <li key={`${name}-${index}`}>{name}</li>)}</ul></div> : null}
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      {request.remainingFiles > 0 && request.remainingBytes > 0 ? <form className="space-y-3" onSubmit={(event) => void submit(event)}><label className="block space-y-2 text-sm font-medium">Choose files<input aria-label="Choose file to upload" type="file" multiple required onChange={(event) => { setFiles(Array.from(event.target.files ?? [])); setError('') }} className="block w-full rounded-md border bg-background p-2 text-sm file:mr-3 file:rounded file:border-0 file:bg-secondary file:px-3 file:py-1.5" /></label>{files.length ? <p className="text-xs text-muted-foreground">{files.length} selected · {formatBytes(files.reduce((total, file) => total + file.size, 0))} total</p> : null}<Button className="w-full" type="submit" disabled={files.length === 0 || sending}>{sending ? 'Uploading…' : `Upload ${files.length > 1 ? `${files.length} files` : 'file'}`}</Button></form> : <p className="text-sm text-muted-foreground">This request has reached its upload limit.</p>}
    </CardContent> : null}
  </Card></main>
}
