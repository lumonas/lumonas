import { shares } from '@/mocks/shares'
import type { FileEntry, RecycleEntry } from '@/api/types'

interface VfsNode {
  id: string
  name: string
  type: 'dir' | 'file'
  sizeBytes: number
  modifiedAt: string
  children?: VfsNode[]
}

const GB = 1_000_000_000
const MB = 1_000_000
const now = Date.now()
const daysAgo = (d: number) => new Date(now - d * 86_400_000).toISOString()

let nodeCounter = 1000
let trashCounter = 100

function dir(name: string, children: VfsNode[] = [], ageDays = 10): VfsNode {
  return { id: `n${++nodeCounter}`, name, type: 'dir', sizeBytes: 0, modifiedAt: daysAgo(ageDays), children }
}

function file(name: string, sizeBytes: number, ageDays = 5): VfsNode {
  return { id: `n${++nodeCounter}`, name, type: 'file', sizeBytes, modifiedAt: daysAgo(ageDays) }
}

function tree(): Record<string, VfsNode> {
  const map: Record<string, VfsNode> = {}
  for (const share of shares) {
    map[share.id] = dir(share.name, [], 1)
  }
  const media = map['share-media']
  if (media?.children) {
    media.children.push(
      dir(
        'Movies',
        [
          file('Dune (2021) [1080p].mkv', 8.4 * GB, 12),
          file('Dune Part Two (2024) [2160p].mkv', 16.1 * GB, 3),
          file('Arrival (2016).mkv', 6.2 * GB, 40),
        ],
        3,
      ),
      dir(
        'TV Shows',
        [
          dir('Severance', [file('S02E01.mkv', 2.1 * GB, 8), file('S02E02.mkv', 2.0 * GB, 8)], 8),
          dir('The Bear', [file('S03E01.mkv', 1.7 * GB, 21)], 21),
        ],
        8,
      ),
      dir(
        'Music',
        [
          file('Album One - FLAC.flac', 412 * MB, 60),
          file('Live Set 2025.mp3', 148 * MB, 30),
        ],
        30,
      ),
      file('readme.txt', 1_200, 90),
    )
  }
  const documents = map['share-documents']
  if (documents?.children) {
    documents.children.push(
      dir('Invoices 2026', [file('invoice-0042.pdf', 84_000, 4), file('invoice-0043.pdf', 81_500, 2)], 2),
      dir('Tax', [file('return-2025.pdf', 1.2 * MB, 120)], 120),
      file('house-insurance.pdf', 2.4 * MB, 15),
      file('budget.xlsx', 44_000, 1),
      file('notes.md', 8_400, 0),
    )
  }
  const photos = map['share-photos']
  if (photos?.children) {
    photos.children.push(
      dir('2025', [file('IMG_5501.jpg', 4.2 * MB, 1), file('IMG_5502.jpg', 3.9 * MB, 1), file('IMG_5510.heic', 2.8 * MB, 0)], 1),
      dir('2024', [file('IMG_4471.jpg', 4.8 * MB, 200), file('IMG_4480.jpg', 5.1 * MB, 195)], 195),
      dir('RAW', [file('IMG_5501.cr3', 28 * MB, 1)], 1),
    )
  }
  const backups = map['share-backups']
  if (backups?.children) {
    backups.children.push(
      file('laptop-full-2026-09-05.zip', 42 * GB, 7),
      file('laptop-full-2026-08-05.zip', 39 * GB, 37),
      file('phone-photos-export.tar', 11 * GB, 2),
    )
  }
  const cloud = map['share-cloud']
  if (cloud?.children) {
    cloud.children.push(
      dir('wallpapers', [file('aurora-4k.png', 8.1 * MB, 45), file('mountains-4k.png', 7.4 * MB, 45)], 45),
      file('recipes.md', 12_000, 9),
    )
  }
  return map
}

export const vfs = tree()

export interface TrashItem extends RecycleEntry {
  node: VfsNode
  parentPath: string
}

export const trash: TrashItem[] = [
  {
    id: 'r101',
    shareId: 'share-media',
    name: 'old-cam-render.mkv',
    originalPath: '/Movies',
    deletedAt: daysAgo(2),
    sizeBytes: 3.2 * GB,
    node: file('old-cam-render.mkv', 3.2 * GB, 300),
    parentPath: '/Movies',
  },
  {
    id: 'r102',
    shareId: 'share-media',
    name: 'sample-clip.mp4',
    originalPath: '/',
    deletedAt: daysAgo(6),
    sizeBytes: 210 * MB,
    node: file('sample-clip.mp4', 210 * MB, 320),
    parentPath: '/',
  },
  {
    id: 'r103',
    shareId: 'share-documents',
    name: 'draft-contract.docx',
    originalPath: '/',
    deletedAt: daysAgo(1),
    sizeBytes: 96_000,
    node: file('draft-contract.docx', 96_000, 25),
    parentPath: '/',
  },
]

function segments(path: string): string[] {
  return path.split('/').filter(Boolean)
}

function getNode(shareId: string, path: string): VfsNode | null {
  const root = vfs[shareId]
  if (!root) return null
  let node = root
  for (const segment of segments(path)) {
    const next = node.children?.find((child) => child.type === 'dir' && child.name === segment)
    if (!next) return null
    node = next
  }
  return node
}

function toEntry(node: VfsNode): FileEntry {
  return {
    id: node.id,
    name: node.name,
    type: node.type,
    sizeBytes: node.type === 'dir' ? dirSize(node) : node.sizeBytes,
    modifiedAt: node.modifiedAt,
  }
}

function dirSize(node: VfsNode): number {
  if (node.type === 'file') return node.sizeBytes
  return (node.children ?? []).reduce((sum, child) => sum + dirSize(child), 0)
}

export function listDir(shareId: string, path: string): FileEntry[] | null {
  const node = getNode(shareId, path)
  if (!node?.children) return null
  return [...node.children]
    .map(toEntry)
    .sort((a, b) =>
      a.type === b.type ? a.name.localeCompare(b.name) : a.type === 'dir' ? -1 : 1,
    )
}

export function mkdir(shareId: string, path: string, name: string): boolean {
  const parent = getNode(shareId, path)
  if (!parent?.children) return false
  if (parent.children.some((child) => child.name === name)) return false
  parent.children.push(dir(name, [], 0))
  parent.modifiedAt = new Date().toISOString()
  return true
}

export function rename(shareId: string, path: string, oldName: string, newName: string): boolean {
  const parent = getNode(shareId, path)
  if (!parent?.children) return false
  const node = parent.children.find((child) => child.name === oldName)
  if (!node) return false
  if (parent.children.some((child) => child.name === newName && child !== node)) return false
  node.name = newName
  node.modifiedAt = new Date().toISOString()
  return true
}

function uniqueName(children: VfsNode[], name: string): string {
  if (!children.some((child) => child.name === name)) return name
  const dot = name.lastIndexOf('.')
  const base = dot > 0 ? name.slice(0, dot) : name
  const ext = dot > 0 ? name.slice(dot) : ''
  let index = 2
  while (children.some((child) => child.name === `${base} (${index})${ext}`)) index++
  return `${base} (${index})${ext}`
}

export function deleteNodes(shareId: string, path: string, names: string[]): number {
  const parent = getNode(shareId, path)
  if (!parent?.children) return 0
  let deleted = 0
  for (const name of names) {
    const node: VfsNode | undefined = parent.children.find((child) => child.name === name)
    if (!node) continue
    parent.children = parent.children.filter((child) => child !== node)
    trash.push({
      id: `r${++trashCounter}`,
      shareId,
      name: node.name,
      originalPath: path,
      deletedAt: new Date().toISOString(),
      sizeBytes: dirSize(node),
      node,
      parentPath: path,
    })
    deleted++
  }
  parent.modifiedAt = new Date().toISOString()
  return deleted
}

export interface TransferInput {
  shareId: string
  sourcePath: string
  names: string[]
  targetShareId: string
  targetPath: string
  op: 'copy' | 'move'
  conflict?: 'overwrite' | 'skip' | 'rename'
}

export function performTransfer(
  input: TransferInput,
): { transferred: number } | { conflicts: string[] } {
  const sourceParent = getNode(input.shareId, input.sourcePath)
  const targetDir = getNode(input.targetShareId, input.targetPath)
  if (!sourceParent?.children || !targetDir?.children) return { transferred: 0 }

  const sources = sourceParent.children.filter((child) => input.names.includes(child.name))
  const existingNames = new Set(targetDir.children.map((child) => child.name))
  const conflicts = sources.filter((source) => existingNames.has(source.name)).map((s) => s.name)
  if (conflicts.length > 0 && !input.conflict) return { conflicts }

  let transferred = 0
  for (const source of sources) {
    let name = source.name
    if (existingNames.has(name)) {
      if (input.conflict === 'skip') continue
      if (input.conflict === 'rename') {
        name = uniqueName(targetDir.children, name)
      } else {
        targetDir.children = targetDir.children.filter((child) => child.name !== name)
      }
    }
    const clone = structuredClone(source)
    clone.name = name
    clone.id = `n${++nodeCounter}`
    clone.modifiedAt = new Date().toISOString()
    if (input.op === 'move') {
      sourceParent.children = sourceParent.children.filter((child) => child !== source)
    }
    targetDir.children.push(clone)
    existingNames.add(name)
    transferred++
  }
  targetDir.modifiedAt = new Date().toISOString()
  return { transferred }
}

export function insertFile(shareId: string, path: string, name: string, sizeBytes: number): string {
  const parent = getNode(shareId, path)
  const finalName = parent?.children ? uniqueName(parent.children, name) : name
  parent?.children?.push(file(finalName, sizeBytes, 0))
  if (parent) parent.modifiedAt = new Date().toISOString()
  return finalName
}

export function listTrash(shareId: string): RecycleEntry[] {
  return trash
    .filter((item) => item.shareId === shareId)
    .map(({ node: _node, parentPath: _parentPath, ...entry }) => entry)
}

export function restoreFromTrash(entryId: string): boolean {
  const index = trash.findIndex((item) => item.id === entryId)
  if (index === -1) return false
  const [item] = trash.splice(index, 1)
  const parent = getNode(item.shareId, item.parentPath) ?? vfs[item.shareId]
  if (!parent?.children) return false
  item.node.name = uniqueName(parent.children, item.node.name)
  parent.children.push(item.node)
  return true
}

export function purgeTrash(entryId?: string, shareId?: string): number {
  const before = trash.length
  if (entryId) {
    const index = trash.findIndex((item) => item.id === entryId)
    if (index !== -1) trash.splice(index, 1)
  } else if (shareId) {
    for (let i = trash.length - 1; i >= 0; i--) {
      if (trash[i].shareId === shareId) trash.splice(i, 1)
    }
  }
  return before - trash.length
}
