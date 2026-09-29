import { create } from 'zustand'

export type LiveConnectionState = 'connecting' | 'live' | 'reconnecting' | 'offline'

interface LiveConnectionStore {
  state: LiveConnectionState
  setState: (state: LiveConnectionState) => void
}

export const useLiveConnection = create<LiveConnectionStore>((set) => ({
  state: 'connecting',
  setState: (state) => set({ state }),
}))
