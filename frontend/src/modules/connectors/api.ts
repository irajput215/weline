// Calls to /api/x/connectors through Weline's shared client. Spec:
// wl-docs/modules/connectors/api.md
import { api } from '@/services/api'

export interface Account {
  provider: string
  status: 'connected' | 'error' | 'not_set_up'
  credentials_set: boolean
  credentials_set_at: string | null
  settings: Record<string, unknown>
  last_test_at: string | null
  last_sync_at: string | null
  last_error: string
}

export interface ProviderInfo {
  name: string
  available: boolean
  fields: { name: string; secret: boolean }[]
}

export interface Sync {
  id: string
  provider: string
  kind: 'test' | 'sync'
  started_at: string
  finished_at: string | null
  outcome: 'ok' | 'error' | 'running'
  counts: Record<string, number>
  error: string
}

const base = '/x/connectors'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const connectorsApi = {
  account: () => data<{ account: Account; providers: ProviderInfo[] }>(api.get(`${base}/account`)),
  save: (body: { provider: string; credentials?: Record<string, string> }) =>
    data<{ account: Account }>(api.put(`${base}/account`, body)),
  test: () => data<{ ok: boolean; message: string; account: Account }>(api.post(`${base}/account/test`)),
  syncs: () => data<{ syncs: Sync[] }>(api.get(`${base}/syncs`))
}
