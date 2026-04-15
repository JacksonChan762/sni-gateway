import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

// Types
export interface Exit {
  id: number
  name: string
  runtime: string
  protocol: string
  provider: string
  country: string
  city: string
  status: string
  public_ip: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Rule {
  id: number
  pattern: string
  type: 'exact' | 'suffix'
  exit_id: number | null
  exit?: Exit
  priority: number
  source: 'manual' | 'subscription'
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Subscription {
  id: number
  name: string
  url: string
  exit_id: number | null
  exit?: Exit
  enabled: boolean
  interval_minutes: number
  last_sync_at: string | null
  last_sync_status: string
  last_error: string
  rule_count: number
  created_at: string
  updated_at: string
}

export interface PublishHistory {
  id: number
  version: string
  status: string
  message: string
  created_at: string
}

export interface Stats {
  rule_count: number
  sub_count: number
  exit_count: number
  running_exit_count: number
  last_publish: PublishHistory
}

// Stats
export const getStats = () => api.get<Stats>('/stats').then(r => r.data)

// Exits
export const getExits = () => api.get<Exit[]>('/exits').then(r => r.data)
export const createExit = (data: Partial<Exit> & { config_yaml?: string; wireguard_private_key?: string; wireguard_addresses?: string; openvpn_user?: string; openvpn_password?: string }) =>
  api.post<Exit>('/exits', data).then(r => r.data)
export const updateExit = (id: number, data: Partial<Exit> & { config_yaml?: string }) =>
  api.put<Exit>(`/exits/${id}`, data).then(r => r.data)
export const deleteExit = (id: number) => api.delete(`/exits/${id}`).then(r => r.data)
export const startExit = (id: number) => api.post(`/exits/${id}/start`).then(r => r.data)
export const stopExit = (id: number) => api.post(`/exits/${id}/stop`).then(r => r.data)
export const restartExit = (id: number) => api.post(`/exits/${id}/restart`).then(r => r.data)
export const testExit = (id: number) => api.post(`/exits/${id}/test`).then(r => r.data)
export const getExitConfig = (id: number) =>
  api.get<{ config_yaml: string }>(`/exits/${id}/config`).then(r => r.data)
export const getExitLogs = (id: number, tail = 100) =>
  api.get<{ logs: string }>(`/exits/${id}/logs?tail=${tail}`).then(r => r.data)
export const getExitStatus = (id: number) =>
  api.get<{ status: string; public_ip: string }>(`/exits/${id}/status`).then(r => r.data)

// Rules
export const getRules = () => api.get<Rule[]>('/rules').then(r => r.data)
export const createRule = (data: Partial<Rule>) => api.post<Rule>('/rules', data).then(r => r.data)
export const updateRule = (id: number, data: Partial<Rule>) =>
  api.put<Rule>(`/rules/${id}`, data).then(r => r.data)
export const deleteRule = (id: number) => api.delete(`/rules/${id}`).then(r => r.data)

// Subscriptions
export const getSubscriptions = () => api.get<Subscription[]>('/subscriptions').then(r => r.data)
export const createSubscription = (data: Partial<Subscription>) =>
  api.post<Subscription>('/subscriptions', data).then(r => r.data)
export const updateSubscription = (id: number, data: Partial<Subscription>) =>
  api.put<Subscription>(`/subscriptions/${id}`, data).then(r => r.data)
export const deleteSubscription = (id: number) =>
  api.delete(`/subscriptions/${id}`).then(r => r.data)
export const syncSubscription = (id: number) =>
  api.post(`/subscriptions/${id}/sync`).then(r => r.data)

// Publish
export const previewConfig = () =>
  api.post<{ config: string }>('/publish/preview').then(r => r.data)
export const validateConfig = () =>
  api.post<{ valid: boolean; error?: string }>('/publish/validate').then(r => r.data)
export const applyConfig = () => api.post('/publish/apply').then(r => r.data)
export const rollbackConfig = () => api.post('/publish/rollback').then(r => r.data)
export const getPublishHistory = () =>
  api.get<PublishHistory[]>('/publish/history').then(r => r.data)
