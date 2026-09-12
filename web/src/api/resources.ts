export interface StorageResource {
  id: string
  label: string
}

export const STORAGE_RESOURCES: StorageResource[] = [
  { id: 'share-media', label: 'Media (share)' },
  { id: 'share-photos', label: 'Photos (share)' },
  { id: 'share-documents', label: 'Documents (share)' },
  { id: 'share-backups', label: 'Backups (share)' },
  { id: 'share-cloud', label: 'Cloud (share)' },
  { id: 'apps', label: 'Apps SSD' },
  { id: 'pool-main', label: 'Main pool' },
]
