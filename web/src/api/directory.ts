// AD 目录集成 API 客户端 (Wave 3 T9)
import api from './client'

export interface GroupMapping {
  id: string
  group_dn: string
  group_name?: string
  role: string
  data_scope: string
  sync_enabled: boolean
  created_at: string
  updated_at: string
}

export interface LDAPStatus {
  enabled: boolean
  host?: string
  port?: number
  connected?: boolean
}

export interface CreateGroupMapping {
  group_dn: string
  group_name?: string
  role: string
  data_scope?: string
}

export interface UpdateGroupMapping {
  role?: string
  data_scope?: string
  sync_enabled?: boolean
  group_name?: string
}

// 组映射 CRUD
export function listGroups(): Promise<GroupMapping[]> {
  return api.get('/admin/ad-groups').then((r) => r.data?.data || [])
}

export function createGroup(data: CreateGroupMapping): Promise<GroupMapping> {
  return api.post('/admin/ad-groups', data).then((r) => r.data?.data)
}

export function updateGroup(id: string, data: UpdateGroupMapping): Promise<GroupMapping> {
  return api.put(`/admin/ad-groups/${id}`, data).then((r) => r.data?.data)
}

export function deleteGroup(id: string): Promise<void> {
  return api.delete(`/admin/ad-groups/${id}`).then((r) => r.data)
}

// LDAP 状态
export function getLDAPStatus(): Promise<LDAPStatus> {
  return api.get('/admin/ldap/status').then((r) => r.data?.data || { enabled: false })
}

// 用户链接/解绑 AD
export function linkAD(userId: string, externalId: string, dn?: string): Promise<void> {
  return api.post(`/admin/users/${userId}/link-ad`, { external_id: externalId, dn }).then((r) => r.data)
}

export function unlinkAD(userId: string): Promise<void> {
  return api.delete(`/admin/users/${userId}/link-ad`).then((r) => r.data)
}
