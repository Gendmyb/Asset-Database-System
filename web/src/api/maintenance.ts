import api from './client'

export interface MaintenanceTicket {
  id: string
  order_no: string
  asset_id: string
  asset_name?: string
  asset_tag?: string
  title: string
  category: string
  status: string
  description?: string
  resolution?: string
  cost?: number
  created_at: string
  updated_at: string
}

export interface CreateMaintenanceData {
  asset_id: string
  title: string
  category: string
  description?: string
}

export interface CompleteMaintenanceData {
  resolution?: string
  cost?: number
}

export interface MaintenanceListParams {
  search?: string
  status?: string
  asset_id?: string
}

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    next_cursor: string | null
    has_more: boolean
  }
}

export function create(data: CreateMaintenanceData): Promise<MaintenanceTicket> {
  return api.post('/maintenance-orders', data).then((r) => r.data)
}

export function list(params?: MaintenanceListParams): Promise<PaginatedResponse<MaintenanceTicket>> {
  return api.get('/maintenance-orders', { params }).then((r) => r.data)
}

export function getById(id: string): Promise<MaintenanceTicket> {
  return api.get(`/maintenance-orders/${id}`).then((r) => r.data?.data || r.data)
}

export function start(id: string): Promise<MaintenanceTicket> {
  return api.post(`/maintenance-orders/${id}/start`).then((r) => r.data)
}

export function complete(id: string, data: CompleteMaintenanceData): Promise<MaintenanceTicket> {
  return api.post(`/maintenance-orders/${id}/complete`, data).then((r) => r.data)
}

export function cancel(id: string): Promise<MaintenanceTicket> {
  return api.post(`/maintenance-orders/${id}/cancel`).then((r) => r.data)
}
