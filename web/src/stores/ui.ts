import { create } from 'zustand'

interface UiState {
  paletteOpen: boolean
  setPaletteOpen: (open: boolean) => void
  togglePalette: () => void
}

export const useUiStore = create<UiState>((set, get) => ({
  paletteOpen: false,
  setPaletteOpen: (open) => set({ paletteOpen: open }),
  togglePalette: () => set({ paletteOpen: !get().paletteOpen }),
}))
