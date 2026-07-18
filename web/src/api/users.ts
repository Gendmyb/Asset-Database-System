import api from './client'

export interface User {
  id: string
  username: string
  role: string
  org_id: string
  email?: string
  disabled?: boolean
  created_at?: string
}

export interface CreateUserData {
  username: string
  password?: string
  role: string
  email?: string
}

export interface UpdateUserData {
  username?: string
  role?: string
  email?: string
  disabled?: boolean
}

export function list(): Promise<User[]> {
  return api.get('/admin/users').then((r) => r.data?.data || r.data)
}

export function get(id: string): Promise<User> {
  return api.get(`/users/${id}`).then((r) => r.data?.data || r.data)
}

export function create(data: CreateUserData): Promise<User> {
  return api.post('/admin/users', data).then((r) => r.data)
}

export function update(id: string, data: UpdateUserData): Promise<User> {
  return api.put(`/admin/users/${id}`, data).then((r) => r.data)
}

export function resetPassword(id: string): Promise<{ new_password: string }> {
  return api.post(`/admin/users/${id}/reset-password`).then((r) => r.data)
}
