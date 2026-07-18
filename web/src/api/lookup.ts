import api from './client'

export interface LookupItem {
  id: string
  name: string
}

export function assetTypes(): Promise<LookupItem[]> {
  return api.get('/lookup/asset-types').then((r) => r.data?.data || r.data)
}

export function locations(): Promise<LookupItem[]> {
  return api.get('/lookup/locations').then((r) => r.data?.data || r.data)
}
