import api from './client'

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  access_token: string
  refresh_token: string
  token_type: string
  user: {
    id: string
    username: string
    role: string
    org_id: string
  }
}

export interface RefreshRequest {
  access_token: string
  refresh_token: string
}

export interface RefreshResponse {
  access_token: string
  refresh_token: string
}

export interface UserProfile {
  id: string
  username: string
  role: string
  org_id: string
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

export function login(data: LoginRequest): Promise<LoginResponse> {
  return api.post('/auth/login', data).then((r) => r.data)
}

export function logout(refreshToken: string): Promise<void> {
  return api.post('/auth/logout', { refresh_token: refreshToken }).then((r) => r.data)
}

export function refresh(data: RefreshRequest): Promise<RefreshResponse> {
  return api.post('/auth/refresh', data).then((r) => r.data)
}

export function me(): Promise<UserProfile> {
  return api.get('/me').then((r) => r.data?.data || r.data)
}

export function changePassword(data: ChangePasswordRequest): Promise<void> {
  return api.put('/me/password', data).then((r) => r.data)
}
