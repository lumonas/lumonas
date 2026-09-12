import { create } from 'zustand'
import { apiPost } from '@/api/client'

const STORAGE_KEY = 'lumonas-auth'

interface AuthState {
  authenticated: boolean
  login: (username: string, password: string) => Promise<boolean>
  logout: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  authenticated: localStorage.getItem(STORAGE_KEY) === '1',
  login: async (username, password) => {
    try {
      await apiPost('/auth/login', { username, password })
      localStorage.setItem(STORAGE_KEY, '1')
      set({ authenticated: true })
      return true
    } catch {
      return false
    }
  },
  logout: async () => {
    try {
      await apiPost('/auth/logout')
    } catch {
      void 0
    }
    localStorage.removeItem(STORAGE_KEY)
    set({ authenticated: false })
  },
}))
