// API 客户端 — JWT 注入 + 401/403 拦截
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

// 响应拦截: 401 直接登出 + 403 处理
// TODO: Phase C 启用 refresh token 逻辑，恢复以下注释代码
// let isRefreshing = false
// let refreshQueue: Array<(token: string) => void> = []

api.interceptors.response.use(
  (res) => res,
  async (error) => {
    // 401 — 暂时直接登出（Phase C 启用 refresh 后再替换）
    if (error.response?.status === 401) {
      localStorage.clear()
      window.location.href = '/login'
      return Promise.reject(error)
    }

    if (error.response?.status === 403) {
      console.warn('403 Forbidden — insufficient permissions')
    }

    return Promise.reject(error)
  }
)

export default api
