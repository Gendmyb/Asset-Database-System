// API 客户端 — JWT 注入 + 401/403 拦截 + Refresh Token 轮换
import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
})

// 请求拦截: JWT 注入
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Refresh token 队列 — 防止并发 refresh 请求
let isRefreshing = false
let refreshQueue: Array<{
  resolve: (token: string) => void
  reject: (err: unknown) => void
}> = []

async function doRefresh(): Promise<string> {
  const refreshToken = localStorage.getItem('refresh_token')
  const accessToken = localStorage.getItem('access_token')
  if (!refreshToken || !accessToken) {
    throw new Error('No credentials')
  }

  const { data } = await axios.post('/api/v1/auth/refresh', {
    access_token: accessToken,
    refresh_token: refreshToken,
  })

  localStorage.setItem('access_token', data.access_token)
  localStorage.setItem('refresh_token', data.refresh_token)

  return data.access_token
}

function clearAuth() {
  localStorage.clear()
  window.location.href = '/login'
}

// 响应拦截: 401 尝试 refresh, 403 记录
api.interceptors.response.use(
  (res) => res,
  async (error) => {
    const originalRequest = error.config

    // 不要对 refresh/login 请求本身重试
    if (originalRequest.url === '/auth/refresh' || originalRequest.url === '/auth/login') {
      clearAuth()
      return Promise.reject(error)
    }

    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true

      // 如果正在刷新, 排队等待
      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          refreshQueue.push({
            resolve: (token: string) => {
              originalRequest.headers.Authorization = `Bearer ${token}`
              resolve(api(originalRequest))
            },
            reject,
          })
        })
      }

      isRefreshing = true

      try {
        const newToken = await doRefresh()

        // 处理队列中的请求
        refreshQueue.forEach(({ resolve }) => resolve(newToken))
        refreshQueue = []

        // 重放原请求
        originalRequest.headers.Authorization = `Bearer ${newToken}`
        return api(originalRequest)
      } catch (refreshErr) {
        // 刷新失败 → 登出
        refreshQueue.forEach(({ reject }) => reject(refreshErr))
        refreshQueue = []
        clearAuth()
        return Promise.reject(refreshErr)
      } finally {
        isRefreshing = false
      }
    }

    if (error.response?.status === 403) {
      console.warn('403 Forbidden — insufficient permissions')
    }

    return Promise.reject(error)
  }
)

export default api
