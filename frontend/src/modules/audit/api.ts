// Calls to /api/x/audit. Spec: wl-docs/modules/audit.
import { api } from '@/services/api'

export interface AuditEvent {
  id: string
  actor: { id: string; name: string } | null
  module: string
  action: string
  entity: string
  entity_id: string
  details: Record<string, unknown>
  at: string
}

const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const auditApi = {
  events: (q: { module?: string; before?: string } = {}) =>
    data<{ events: AuditEvent[]; next_before: string | null }>(
      api.get('/x/audit/events', { params: Object.fromEntries(Object.entries(q).filter(([, v]) => v)) })
    )
}
