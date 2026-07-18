import api from './client'

export interface Asset {
  id: string
  asset_tag: string
  name: string
  type_id: string
  org_id: string
  serial_number?: string
  manufacturer?: string
  model?: string
  lifecycle_state: string
  status: string
  properties: Record<string, unknown>
  version: number
  created_at: string
  updated_at: string
}

export interface AssetListParams {
  search?: string
  status?: string
  lifecycle?: string
  manufacturer?: string
  cursor?: string
  limit?: number
}

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    next_cursor: string | null
    has_more: boolean
  }
  request_id: string
}

export interface CreateAssetData {
  name: string
  type_id?: string
  serial_number?: string
  manufacturer?: string
  model?: string
  asset_tag?: string
  managed_by?: string
  location_id?: string
  price?: number
  purchase_date?: string
  supplier?: string
  warranty_expiry?: string
}

export interface UpdateAssetData {
  name?: string
  manufacturer?: string
  model?: string
  serial_number?: string
  status?: string
}

export interface AssignAssetData {
  assigned_to: string
  notes?: string
}

export interface BorrowAssetData {
  assigned_to: string
  notes?: string
  due_date: string
}

export interface BatchData {
  asset_ids: string[]
  action: 'assign' | 'transition' | 'delete'
  params?: Record<string, unknown>
}

export function list(params?: AssetListParams): Promise<PaginatedResponse<Asset>> {
  return api.get('/assets', { params }).then((r) => r.data)
}

export function getById(id: string): Promise<Asset> {
  return api.get(`/assets/${id}`).then((r) => r.data?.data || r.data)
}

export function create(data: CreateAssetData): Promise<Asset> {
  return api.post('/assets', data).then((r) => r.data)
}

export function update(id: string, data: UpdateAssetData, version: number): Promise<Asset> {
  return api.put(`/assets/${id}`, data, {
    headers: { 'If-Match': String(version) },
  }).then((r) => r.data)
}

export function remove(id: string): Promise<void> {
  return api.delete(`/assets/${id}`).then((r) => r.data)
}

export function transition(id: string, to: string): Promise<Asset> {
  return api.post(`/assets/${id}/transition`, { to }).then((r) => r.data)
}

export function assign(id: string, data: AssignAssetData): Promise<unknown> {
  return api.post(`/assets/${id}/assign`, data).then((r) => r.data)
}

export function release(id: string): Promise<unknown> {
  return api.post(`/assets/${id}/release`).then((r) => r.data)
}

export function borrow(id: string, data: BorrowAssetData): Promise<unknown> {
  return api.post(`/assets/${id}/borrow`, data).then((r) => r.data)
}

export function batch(data: BatchData): Promise<unknown> {
  return api.post('/assets/batch', data).then((r) => r.data)
}
