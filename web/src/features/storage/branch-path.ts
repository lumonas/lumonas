// Mirrors the canonical backend branch path (/srv/disks/<sanitized stable id>).
export function diskBranchPath(diskId: string): string {
  const segment = diskId.replace(/[^A-Za-z0-9._-]/g, '_') || 'unknown'
  return `/srv/disks/${segment}`
}
