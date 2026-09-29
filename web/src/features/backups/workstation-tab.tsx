import { Link } from 'react-router-dom'
import { ArrowUpRight, HardDriveUpload, KeyRound, RefreshCw, ShieldCheck } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const binaryNames = [
  'linux_amd64',
  'linux_arm64',
  'windows_amd64.exe',
  'darwin_amd64',
  'darwin_arm64',
]

export function WorkstationTab() {
  return <div className="grid gap-4 xl:grid-cols-2">
    <Card className="xl:col-span-2">
      <CardHeader><CardTitle className="flex items-center gap-2 text-sm"><HardDriveUpload className="size-4" />Encrypted workstation backups</CardTitle></CardHeader>
      <CardContent className="space-y-3 text-sm text-muted-foreground">
        <p>Run the small LumoNAS client on Windows, macOS, or Linux to create encrypted point-in-time archives of a folder. Uploads resume after network interruptions, and restores extract to a local folder without replacing existing files by default.</p>
        <p>Encryption happens on the workstation. LumoNAS stores the archive in a dedicated share and cannot decrypt it. Keep the encryption key somewhere separate from the NAS and the backed-up computer.</p>
        <div className="flex flex-wrap gap-2"><Button asChild size="sm"><Link to="/settings?tab=security"><KeyRound />Create a workstation token<ArrowUpRight /></Link></Button></div>
      </CardContent>
    </Card>
    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2 text-sm"><ShieldCheck className="size-4" />One-time setup</CardTitle></CardHeader>
      <CardContent className="space-y-3 text-sm text-muted-foreground">
        <ol className="list-decimal space-y-2 pl-5">
          <li>Create a private share for workstation archives.</li>
          <li>In Settings → Security, create an API token with “Back up and restore one workstation” and bind it to that share. Copy the token when it is shown; LumoNAS will not show it again. The permission string displayed with the token includes the share ID for the client environment variable.</li>
          <li>Download the matching client from the LumoNAS release assets. The client builds are named for Linux, Windows, and macOS on x64 or ARM64.</li>
          <li>Generate a 32-byte encryption key with <code className="rounded bg-muted px-1">lumonas-workstation keygen</code>. Store it in a password manager or other recovery location.</li>
        </ol>
        <p>The token can upload and download archive files on its selected share only. Revoke it from Settings if the workstation is lost.</p>
      </CardContent>
    </Card>
    <Card>
      <CardHeader><CardTitle className="text-sm">Run and restore</CardTitle></CardHeader>
      <CardContent className="space-y-3 text-sm text-muted-foreground">
        <p>Set these environment variables for the client, then schedule the backup command with the workstation’s normal scheduler.</p>
        <p>By default, the whole source folder is archived. Repeat <code className="rounded bg-muted px-1">--include</code> to select relative paths and <code className="rounded bg-muted px-1">--exclude</code> to omit private or regenerable files; selection patterns cannot escape the source folder.</p>
        <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 text-xs text-foreground">{`LUMONAS_SERVER_URL=https://nas.example.net
LUMONAS_API_TOKEN=<share-bound token>
LUMONAS_WORKSTATION_SHARE_ID=<selected share ID>
LUMONAS_BACKUP_KEY=<32-byte base64 key>

lumonas-workstation backup --source "$HOME" --include Documents --include Projects --exclude '*/node_modules' --exclude '*.tmp'
lumonas-workstation list
lumonas-workstation restore --file <archive>.lwb --to "$HOME/Documents-restored"`}</pre>
        <p>Use <code className="rounded bg-muted px-1">restore --overwrite</code> only when you intend to replace files. Symlinks are skipped during backup; restores reject path traversal and symlinked destination parents.</p>
      </CardContent>
    </Card>
    <Card className="xl:col-span-2">
      <CardHeader><CardTitle className="flex items-center gap-2 text-sm"><RefreshCw className="size-4" />Keep a workstation folder in sync</CardTitle></CardHeader>
      <CardContent className="space-y-3 text-sm text-muted-foreground">
        <p>Use <code className="rounded bg-muted px-1">sync</code> for one-way workstation-to-NAS copies. Schedule it with your normal OS scheduler to keep the selected folder current. NAS files are never deleted; first-run name collisions and remote edits are reported instead of overwritten.</p>
        <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 text-xs text-foreground">{`lumonas-workstation sync --source "$HOME/Documents" --path Workstation/Documents --dry-run
lumonas-workstation sync --source "$HOME/Documents" --path Workstation/Documents --exclude '*.tmp'
lumonas-workstation sync --source "$HOME/Documents" --path Workstation/Documents --overwrite-existing`}</pre>
        <p>Review the dry run first. Sync keeps a local baseline so it can detect remote changes; use <code className="rounded bg-muted px-1">--overwrite-existing</code> only after reviewing conflicts. Sync transfers files over HTTPS but does not encrypt their contents; use encrypted <code className="rounded bg-muted px-1">backup</code> archives for client-side encryption.</p>
      </CardContent>
    </Card>
    <Card className="xl:col-span-2">
      <CardHeader><CardTitle className="text-sm">Client builds</CardTitle></CardHeader>
      <CardContent className="flex flex-wrap gap-2">{binaryNames.map((name) => <code key={name} className="rounded border bg-muted/40 px-2 py-1 text-xs">lumonas-workstation_{'<version>'}_{name}</code>)}</CardContent>
    </Card>
  </div>
}
