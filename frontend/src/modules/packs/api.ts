// Calls to /api/x/packs. Spec: wl-docs/modules/packs/api.md
import { api } from '@/services/api'

export type ItemStatus = 'created' | 'exists' | 'kept' | 'waiting_approval' | 'needs_whatsapp' | 'failed'

export interface InstallItem {
  kind: 'template' | 'ai' | 'segment' | 'reminder'
  name: string
  status: ItemStatus
  message?: string
}

export interface InstallResult {
  pack_id: string
  version: number
  items: InstallItem[]
  installed_at: string
}

export interface PackSummary {
  id: string
  name: string
  description: string
  version: number
  recommended: boolean
  installed: InstallResult | null
  counts: Record<'templates' | 'segments' | 'reminders' | 'flows', number>
}

export interface Pack {
  id: string
  name: string
  description: string
  connectors: string[]
  templates: { name: string; category: string; body: string; buttons?: string[] }[]
  ai: { tone: string; instructions: string; emergency_words: string[] }
  segments: { name: string; description: string }[]
  reminders: { kind: string; name: string; offset_minutes?: number; recall_days?: number; template: string }[]
  flows: { name: string; guide: string }[]
}

const base = '/x/packs'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const packsApi = {
  list: () => data<{ packs: PackSummary[] }>(api.get(base)),
  get: (id: string) => data<{ pack: Pack }>(api.get(`${base}/${id}`)),
  install: (id: string) => data<{ result: InstallResult }>(api.post(`${base}/${id}/install`, {}))
}
