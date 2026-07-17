// 资产类型定义
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

export interface PaginatedResponse<T> {
  data: T[]
  pagination: {
    next_cursor: string | null
    has_more: boolean
  }
  request_id: string
}

export interface User {
  id: string
  username: string
  role: string
  org_id: string
}

// Agent 类型定义
export interface Agent {
  id: string
  name: string
  hostname: string
  org_id: string
  status: 'online' | 'offline' | 'error' | 'degraded'
  last_heartbeat: string | null
  version: string
  metadata: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface AgentHealthStats {
  total: number
  online: number
  offline: number
  error: number
  degraded: number
}
