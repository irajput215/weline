// Calls to /api/x/ads through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it). Spec:
// wl-docs/modules/ads/api.md
import { api } from '@/services/api'

export const CHANNELS = ['meta_ad', 'google_ad', 'website', 'print', 'email', 'other'] as const
export type Channel = (typeof CHANNELS)[number]

export interface Stats {
  conversations: number
  new_contacts: number
  opted_in: number
  booked: number
}

export interface EntryPoint {
  id: string
  name: string
  code: string
  channel: Channel
  campaign: string
  wa_number: string
  prefill_text: string
  link: string
  tag: string
  tag_campaign: boolean
  campaign_tag: string
  archived_at: string | null
  stats: Stats
  created_at: string
  updated_at: string
}

export interface CampaignTotals extends Stats {
  campaign: string
}

export interface Listing {
  entrypoints: EntryPoint[]
  campaigns: CampaignTotals[]
  period: { from: string; to: string }
}

export interface Arrival {
  id: string
  contact: { id: string; name: string; phone_masked: string }
  arrived_at: string
  is_new: boolean
  opted_in: boolean
  booked: boolean
}

export interface EntryPointInput {
  name: string
  code?: string
  channel: Channel
  campaign: string
  wa_number: string
  prefill_text: string
  tag: string
  tag_campaign: boolean
}

export interface PeriodQuery {
  from?: string
  to?: string
}

const base = '/x/ads/entrypoints'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)
const clean = (q: Record<string, string | undefined>) =>
  ({ params: Object.fromEntries(Object.entries(q).filter(([, v]) => v !== undefined && v !== '')) })

export const adsApi = {
  list: (q: PeriodQuery & { include_archived?: boolean } = {}) =>
    data<Listing>(api.get(base, clean({ from: q.from, to: q.to, include_archived: q.include_archived ? 'true' : undefined }))),
  get: (id: string, q: PeriodQuery = {}) => data<{ entrypoint: EntryPoint }>(api.get(`${base}/${id}`, clean({ ...q }))),
  create: (body: EntryPointInput) => data<{ entrypoint: EntryPoint }>(api.post(base, body)),
  update: (id: string, body: EntryPointInput) => data<{ entrypoint: EntryPoint }>(api.put(`${base}/${id}`, body)),
  archive: (id: string) => data<{ entrypoint: EntryPoint }>(api.post(`${base}/${id}/archive`)),
  remove: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/${id}`)),
  arrivals: (id: string, before?: string) =>
    data<{ arrivals: Arrival[]; next_before: string | null }>(api.get(`${base}/${id}/arrivals`, clean({ before })))
}
