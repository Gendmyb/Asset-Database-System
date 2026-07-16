// Zustand Auth Store — JWT 管理
import { create } from 'zustand'

interface AuthState {
  token: string | null
  refreshToken: string | null
  user: { id: string; username: string; role: string } | null
  isAuthenticated: boolean
  login: (accessToken: string, refreshToken: string, user: AuthState['user']) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('access_token'),
  refreshToken: localStorage.getItem('refresh_token'),
  user: JSON.parse(localStorage.getItem('user') || 'null'),
  isAuthenticated: !!localStorage.getItem('access_token'),

  login: (accessToken, refreshToken, user) => {
    localStorage.setItem('access_token', accessToken)
    localStorage.setItem('refresh_token', refreshToken)
    localStorage.setItem('user', JSON.stringify(user))
    set({ token: accessToken, refreshToken, user, isAuthenticated: true })
  },

  logout: () => {
    localStorage.clear()
    set({ token: null, refreshToken: null, user: null, isAuthenticated: false })
  },
}))
