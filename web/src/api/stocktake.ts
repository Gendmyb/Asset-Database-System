import api from './client'

export interface StocktakePlan {
  id: string
  plan_no?: string
  name: string
  status: 'draft' | 'in_progress' | 'completed' | 'canceled'
  scope_location_id?: string
  scope_type_id?: string
  created_at: string
  started_at?: string
  completed_at?: string
}

export interface StocktakeItem {
  id: string
  plan_id: string
  asset_id: string
  asset_name?: string
  asset_tag?: string
  expected_location?: string
  expected_status?: string
  result?: 'found' | 'missing' | 'moved'
  actual_location_id?: string
  actual_location_name?: string
  notes?: string
  checked_by?: string
  created_at: string
}

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    next_cursor: string | null
    has_more: boolean
  }
  request_id: string
}

export interface StocktakeReport {
  plan: StocktakePlan
  summary: {
    total: number
    found: number
    missing: number
    moved: number
    surplus: number
    progress: number
  }
  items: StocktakeItem[]
  surplus_items: Array<{
    id: string
    surplus_note: string
    created_at: string
  }>
}

// ── Plans ──────────────────────────────────────────────

export function createPlan(data: {
  name: string
  scope_location_id?: string
  scope_type_id?: string
}): Promise<StocktakePlan> {
  return api.post('/stocktakes', data).then((r) => r.data?.data || r.data)
}

export function listPlans(params?: {
  status?: string
  cursor?: string
  limit?: number
}): Promise<PaginatedResponse<StocktakePlan>> {
  return api.get('/stocktakes', { params }).then((r) => r.data)
}

export function getPlan(id: string): Promise<StocktakePlan> {
  return api.get(`/stocktakes/${id}`).then((r) => r.data?.data || r.data)
}

export function startPlan(id: string): Promise<StocktakePlan> {
  return api.post(`/stocktakes/${id}/start`).then((r) => r.data?.data || r.data)
}

export function completePlan(
  id: string,
  data: { apply_moves: boolean }
): Promise<StocktakePlan> {
  return api
    .post(`/stocktakes/${id}/complete`, data)
    .then((r) => r.data?.data || r.data)
}

// ── Items ──────────────────────────────────────────────

export function listItems(
  planId: string,
  params?: {
    result?: string
    search?: string
    cursor?: string
    limit?: number
  }
): Promise<PaginatedResponse<StocktakeItem>> {
  return api
    .get(`/stocktakes/${planId}/items`, { params })
    .then((r) => r.data)
}

export function updateItem(
  planId: string,
  itemId: string,
  data: {
    result?: string
    actual_location_id?: string
    notes?: string
  }
): Promise<StocktakeItem> {
  return api
    .put(`/stocktakes/${planId}/items/${itemId}`, data)
    .then((r) => r.data?.data || r.data)
}

export function addSurplus(
  planId: string,
  data: { surplus_note: string }
): Promise<unknown> {
  return api
    .post(`/stocktakes/${planId}/items`, data)
    .then((r) => r.data)
}

// ── Report ─────────────────────────────────────────────

export function getReport(
  planId: string,
  format?: 'json' | 'csv'
): Promise<StocktakeReport> {
  return api
    .get(`/stocktakes/${planId}/report`, {
      params: format ? { format } : undefined,
    })
    .then((r) => r.data?.data || r.data)
}

export function exportReport(planId: string): Promise<Blob> {
  return api
    .get(`/stocktakes/${planId}/report/export`, { responseType: 'blob' })
    .then((r) => r.data)
}
