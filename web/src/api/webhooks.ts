// Webhook 端点管理 API — 对应后端 /admin/webhooks (admin+)
import api from './client'

export interface WebhookEndpoint {
  id: string
  org_id: string
  url: string
  events: string[]
  active: boolean
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: string
  endpoint_id: string
  event_type: string
  status: string
  attempts: number
  status_code?: number
  last_error?: string
  created_at: string
}

export interface CreateWebhookData {
  url: string
  secret?: string
  events: string[]
  active?: boolean
}

export interface UpdateWebhookData {
  url?: string
  events?: string[]
  active?: boolean
}

export function list(): Promise<WebhookEndpoint[]> {
  return api.get('/admin/webhooks').then((r) => r.data?.data || r.data)
}

export function get(id: string): Promise<WebhookEndpoint> {
  return api.get(`/admin/webhooks/${id}`).then((r) => r.data?.data || r.data)
}

export function create(data: CreateWebhookData): Promise<{ id: string }> {
  return api.post('/admin/webhooks', data).then((r) => r.data?.data || r.data)
}

export function update(id: string, data: UpdateWebhookData): Promise<void> {
  return api.put(`/admin/webhooks/${id}`, data).then((r) => r.data)
}

export function remove(id: string): Promise<void> {
  return api.delete(`/admin/webhooks/${id}`).then((r) => r.data)
}

export function listDeliveries(id: string): Promise<WebhookDelivery[]> {
  return api.get(`/admin/webhooks/${id}/deliveries`).then((r) => r.data?.data || r.data)
}
