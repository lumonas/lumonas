import { create } from 'zustand'
import type { LogLine } from '@/api/types'

const MAX_LINES = 1000

interface LogsState {
  lines: LogLine[]
  seeded: Set<string>
  append: (line: LogLine) => void
  seed: (container: string, lines: LogLine[]) => void
}

export const useLogsStore = create<LogsState>((set) => ({
  lines: [],
  seeded: new Set<string>(),
  append: (line) =>
    set((state) => ({ lines: [...state.lines.slice(-(MAX_LINES - 1)), line] })),
  seed: (container, seedLines) =>
    set((state) => {
      if (state.seeded.has(container)) return state
      const seeded = new Set(state.seeded)
      seeded.add(container)
      return { lines: [...state.lines, ...seedLines], seeded }
    }),
}))
