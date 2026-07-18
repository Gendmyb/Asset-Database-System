import api from './client'

export interface ReportsSummary {
  total_assets: number
  total_purchase_price: number
  total_depreciation: number
  net_book_value: number
  by_status?: Record<string, number>
  recent_additions?: number
}

export interface DepreciationItem {
  asset_name: string
  asset_tag: string
  purchase_price: number
  monthly_depreciation: number
  depreciated_months: number
  accumulated_depreciation: number
  net_book_value: number
}

export interface MaintenanceCostItem {
  asset_name: string
  asset_tag: string
  total_cost: number
  maintenance_count: number
  last_maintenance?: string
}

export interface AssignmentDueItem {
  asset_name: string
  asset_tag: string
  assigned_to: string
  due_date: string
  days_overdue: number
}

export function summary(): Promise<ReportsSummary> {
  return api.get('/reports/summary').then((r) => r.data?.data || r.data)
}

export function depreciation(params?: { search?: string }): Promise<DepreciationItem[]> {
  return api.get('/reports/depreciation', { params }).then((r) => r.data?.data || r.data)
}

export function maintenanceCost(params?: { start_date?: string; end_date?: string }): Promise<MaintenanceCostItem[]> {
  return api.get('/reports/maintenance-cost', { params }).then((r) => r.data?.data || r.data)
}

export function assignmentsDue(days?: number): Promise<AssignmentDueItem[]> {
  return api.get('/reports/assignments-due', { params: { days: days ?? 30 } }).then((r) => r.data?.data || r.data)
}

export function exportDepreciation(): Promise<Blob> {
  return api
    .get('/reports/depreciation/export', { responseType: 'blob' })
    .then((r) => r.data)
}
