// Calls to /api/x/billing. Spec: wl-docs/modules/billing/api.md
import { api } from '@/services/api'

export type Metric = 'broadcast_recipients' | 'ai_runs' | 'contacts' | 'users' | 'whatsapp_numbers' | 'connected_systems'

export interface Usage {
  metric: Metric
  used: number
  limit: number // -1: no limit
  monthly: boolean
  enforced: boolean
}

export interface PlanInfo {
  key: 'trial' | 'starter' | 'clinic' | 'group'
  name: string
  limits: Record<Metric, number>
  available: boolean
}

export interface Summary {
  enabled: boolean
  stripe_ready: boolean
  plan: string
  effective_plan: string
  status: string
  trial_ends_at: string | null
  current_period_end: string | null
  cancel_at_period_end: boolean
  has_customer: boolean
  plans: PlanInfo[] | null
  usage: Usage[]
  meta: { counts: Record<string, number>; pricing_url: string }
}

export interface Invoice {
  id: string
  number: string
  created: string
  total: number
  currency: string
  status: string
  hosted_url: string
}

const base = '/x/billing'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const billingApi = {
  summary: () => data<Summary>(api.get(`${base}/summary`)),
  invoices: () => data<{ invoices: Invoice[] }>(api.get(`${base}/invoices`)),
  checkout: (plan: string) => data<{ url: string }>(api.post(`${base}/checkout`, { plan })),
  portal: () => data<{ url: string }>(api.post(`${base}/portal`, {}))
}
