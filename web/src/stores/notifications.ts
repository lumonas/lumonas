import { create } from 'zustand'

interface NotificationsState {
  readIds: Set<string>
  markRead: (id: string) => void
  markAllRead: (ids: string[]) => void
  isRead: (id: string) => boolean
}

export const useNotificationsStore = create<NotificationsState>((set, get) => ({
  readIds: new Set<string>(),
  markRead: (id) =>
    set((state) => {
      const next = new Set(state.readIds)
      next.add(id)
      return { readIds: next }
    }),
  markAllRead: (ids) =>
    set(() => ({ readIds: new Set(ids) })),
  isRead: (id) => get().readIds.has(id),
}))
