import {
  Archive,
  File as FileIcon,
  FileText,
  Film,
  Folder,
  Image as ImageIcon,
  Music,
  type LucideIcon,
} from 'lucide-react'
import type { FileEntry } from '@/api/types'

const EXT_ICONS: Record<string, LucideIcon> = {
  mkv: Film,
  mp4: Film,
  avi: Film,
  mov: Film,
  jpg: ImageIcon,
  jpeg: ImageIcon,
  png: ImageIcon,
  heic: ImageIcon,
  webp: ImageIcon,
  cr3: ImageIcon,
  mp3: Music,
  flac: Music,
  wav: Music,
  m4a: Music,
  zip: Archive,
  tar: Archive,
  gz: Archive,
  '7z': Archive,
  pdf: FileText,
  docx: FileText,
  xlsx: FileText,
  md: FileText,
  txt: FileText,
}

export function entryIcon(entry: FileEntry): LucideIcon {
  if (entry.type === 'dir') return Folder
  const ext = entry.name.split('.').pop()?.toLowerCase() ?? ''
  return EXT_ICONS[ext] ?? FileIcon
}

export function typeLabel(entry: FileEntry): string {
  if (entry.type === 'dir') return 'Folder'
  const ext = entry.name.split('.').pop()?.toLowerCase() ?? ''
  return ext ? ext.toUpperCase() : 'File'
}
