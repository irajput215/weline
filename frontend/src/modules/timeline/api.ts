// Calls to /api/x/timeline. Spec: wl-docs/modules/timeline/api.md
import { api } from '@/services/api'

export interface ContactRef {
  id: string
  name: string
  phone_masked: string
  created_at: string
}

export type Kind = 'message_in' | 'message_out' | 'broadcast' | 'appointment' | 'reminder' | 'consent' | 'ad_arrival' | 'ai' | 'handover'

export interface Item {
  kind: Kind
  at: string
  fields: Record<string, string>
}

export interface Timeline {
  contact: ContactRef
  items: Item[]
  next_before: string | null
  appointments_unavailable: boolean
}

const base = '/x/timeline'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const timelineApi = {
  search: (q: string) => data<{ contacts: ContactRef[] }>(api.get(`${base}/contacts`, { params: { q } })),
  get: (id: string, before?: string) => data<Timeline>(api.get(`${base}/contacts/${id}`, { params: before ? { before } : {} }))
}
