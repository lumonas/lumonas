import { create } from 'zustand'

export type Density = 'comfortable' | 'compact'

interface UiState {
  paletteOpen: boolean
  density: Density
  setPaletteOpen: (open: boolean) => void
  togglePalette: () => void
  setDensity: (density: Density) => void
}

const DENSITY_KEY = 'lumonas-density'

export const useUiStore = create<UiState>((set, get) => ({
  paletteOpen: false,
  density: localStorage.getItem(DENSITY_KEY) === 'compact' ? 'compact' : 'comfortable',
  setPaletteOpen: (open) => set({ paletteOpen: open }),
  togglePalette: () => set({ paletteOpen: !get().paletteOpen }),
  setDensity: (density) => {
    localStorage.setItem(DENSITY_KEY, density)
    set({ density })
  },
}))
