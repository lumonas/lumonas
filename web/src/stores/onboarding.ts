import { create } from 'zustand'

const FLAG_KEY = 'lumonas-onboarded'
const PROGRESS_KEY = 'lumonas-onboarding-progress'

interface OnboardingStoreState {
  done: boolean
  complete: () => void
}

export const useOnboardingStore = create<OnboardingStoreState>((set) => ({
  done: localStorage.getItem(FLAG_KEY) === '1',
  complete: () => {
    localStorage.setItem(FLAG_KEY, '1')
    localStorage.removeItem(PROGRESS_KEY)
    set({ done: true })
  },
}))

export function loadProgress<T>(): T | null {
  try {
    const raw = localStorage.getItem(PROGRESS_KEY)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    return null
  }
}

export function saveProgress(state: unknown) {
  try {
    localStorage.setItem(PROGRESS_KEY, JSON.stringify(state))
  } catch {
    void 0
  }
}
