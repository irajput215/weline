// Calls to /api/x/consent through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it). Spec:
// wl-docs/modules/consent/api.md
import { api } from '@/services/api'

export type Action = 'opt_in' | 'opt_out'
export type Source = 'keyword' | 'button' | 'staff' | 'import' | 'meta' | 'ad' | 'web_form'
export type Marketing = 'opted_in' | 'opted_out' | 'not_recorded'

export interface ContactRef {
  id: string
  name: string
  phone_masked: string
}

export interface ConsentEvent {
  id: string
  contact: ContactRef
  action: Action
  source: Source
  previous: 'opted_in' | 'opted_out' | null
  given_at: string
  recorded_at: string
  recorded_by: { id: string; name: string } | null
  keyword: string
  evidence: string
  import_id: string | null
}

export interface Summary {
  opted_in: number
  opted_out: number
  not_recorded: number
  total_contacts: number
  require_opt_in: boolean
}

export interface FoundContact extends ContactRef {
  marketing: Marketing
}

export interface ContactConsent {
  contact: ContactRef
  marketing: Marketing
  source: Source | null
  given_at: string | null
  events: ConsentEvent[]
}

export interface Settings {
  opt_out_keywords: string[]
  opt_in_keywords: string[]
  opt_in_phrases: string[]
  opt_out_reply: string
  opt_in_reply: string
  opt_out_tag: string
  require_opt_in: boolean
  service_after_stop: 'continue' | 'stop'
}

export interface ImportPreview {
  token: string
  rows_total: number
  will_apply: number
  already_opted_in: number
  unknown_numbers: number
  invalid: number
  invalid_rows: { row: number; reason: string }[]
  opted_out_later: number
}

export interface ImportResult {
  id: string
  applied: number
  skipped_unknown: number
  skipped_invalid: number
  skipped_opted_out_later: number
  already_opted_in: number
}

export interface EventQuery {
  action?: Action | ''
  source?: Source | ''
  from?: string
  to?: string
  contact_id?: string
  before?: string
}

const base = '/x/consent'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)
const clean = (q: Record<string, string | undefined>) =>
  ({ params: Object.fromEntries(Object.entries(q).filter(([, v]) => v !== undefined && v !== '')) })

export const consentApi = {
  summary: () => data<Summary>(api.get(`${base}/summary`)),
  events: (q: EventQuery = {}) =>
    data<{ events: ConsentEvent[]; next_before: string | null }>(api.get(`${base}/events`, clean({ ...q }))),
  search: (q: string) => data<{ contacts: FoundContact[] }>(api.post(`${base}/contacts/search`, { q })),
  contact: (id: string) => data<ContactConsent>(api.get(`${base}/contacts/${id}`)),
  record: (id: string, body: { action: Action; given_at?: string; evidence?: string }) =>
    data<{ marketing: Marketing; changed: boolean; recorded: boolean; contact: ContactConsent }>(api.post(`${base}/contacts/${id}`, body)),
  previewImport: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return data<ImportPreview>(api.post(`${base}/imports/preview`, form, { headers: { 'Content-Type': 'multipart/form-data' } }))
  },
  applyImport: (body: { token: string; filename: string; evidence: string; confirm_count: number }) =>
    data<{ import: ImportResult }>(api.post(`${base}/imports`, body)),
  settings: () => data<{ settings: Settings; defaults: Settings }>(api.get(`${base}/settings`)),
  saveSettings: (body: Partial<Settings>) => data<{ settings: Settings }>(api.put(`${base}/settings`, body))
}
