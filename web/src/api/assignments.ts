import api from './client'

export interface Assignment {
  id: string
  asset_id: string
  assigned_by: string
  assigned_to: string
  org_id: string
  notes?: string
  due_date?: string
  returned_at?: string
  created_at: string
}

export interface AssignmentListParams {
  asset_id?: string
  assigned_to?: string
  status?: string
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

export function list(params?: AssignmentListParams): Promise<PaginatedResponse<Assignment>> {
  return api.get('/assignments', { params }).then((r) => r.data)
}

export function getByAsset(assetId: string): Promise<Assignment> {
  return api.get(`/assets/${assetId}/assignments`).then((r) => r.data?.data || r.data)
}

export function getByUser(userId: string): Promise<Assignment[]> {
  return api.get(`/users/${userId}/assignments`).then((r) => r.data?.data || r.data)
}
