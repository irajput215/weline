// Calls to /api/x/operator (Weline staff only). Spec: wl-docs/modules/operator/api.md
import { api } from '@/services/api'

export interface Tenant {
  id: string
  name: string
  created_at: string
  source: 'signup' | 'existing'
  vertical: string
  plan: string
  plan_status: string
  trial_ends_at: string | null
  members: number
  contacts: number
  whatsapp_numbers: number
  connector: string
  connector_status: string
  last_message_at: string | null
}

export interface TenantDetail extends Tenant {
  owner: { name: string; email: string } | null
  health: {
    failed_messages: number
    failed_reminders: number
    skipped_reminders: number
    ai_errors: number
    connector_error: string
    connector_last_sync_at: string | null
  }
  usage: { broadcast_recipients: number; ai_runs: number }
  setup_done: number
  has_stripe_customer: boolean
}

export interface Overview {
  organizations: number
  signups_7d: number
  signups_30d: number
  by_plan: Record<string, number>
  connector_errors: number
  with_failed_sends_7d: number
}

const base = '/x/operator'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const operatorApi = {
  overview: () => data<Overview>(api.get(`${base}/overview`)),
  tenants: (q: string, offset = 0) =>
    data<{ tenants: Tenant[]; next_offset: number | null }>(api.get(`${base}/tenants`, { params: { q: q || undefined, offset: offset || undefined } })),
  tenant: (id: string) => data<TenantDetail>(api.get(`${base}/tenants/${id}`))
}
