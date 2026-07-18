import api from './client'

export interface Settings {
  [key: string]: string
}

export function get(): Promise<Settings> {
  return api.get('/settings').then((r) => r.data?.data || r.data)
}

export function set(key: string, value: string): Promise<void> {
  return api.put('/settings', { key, value }).then((r) => r.data)
}

export function nextTag(): Promise<{ asset_tag: string }> {
  return api.get('/settings/next-tag').then((r) => r.data)
}
