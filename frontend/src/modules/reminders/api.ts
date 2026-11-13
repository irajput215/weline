// Calls to /api/x/reminders. Spec: wl-docs/modules/reminders/api.md
import { api, templatesService } from '@/services/api'

export type Kind = 'reminder' | 'recall'
export type Variable = 'name' | 'service' | 'when' | 'location' | 'practitioner'

export interface Rule {
  id?: string
  kind: Kind
  name: string
  offset_minutes: number
  recall_days: number
  template_name: string
  language: string
  variables: Variable[]
  service_ids: string[]
  enabled: boolean
  updated_at?: string
}

export interface Send {
  id: string
  rule_name: string
  kind: Kind
  start_at: string
  status: 'pending' | 'sent' | 'failed' | 'skipped'
  reason: string
  reply: '' | 'confirmed' | 'cancelled' | 'reschedule'
  contact_id: string | null
  created_at: string
}

export interface Due {
  customer_id: string
  name: string
  service: string
  last_visit: string
}

export interface TemplateOption {
  name: string
  language: string
}

export type SendCounts = Record<'sent' | 'skipped' | 'not_due' | 'already', number>

const base = '/x/reminders'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const remindersApi = {
  rules: () => data<{ rules: Rule[] }>(api.get(`${base}/rules`)),
  createRule: (body: Rule) => data<{ rule: Rule }>(api.post(`${base}/rules`, body)),
  updateRule: (id: string, body: Rule) => data<{ rule: Rule }>(api.put(`${base}/rules/${id}`, body)),
  deleteRule: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/rules/${id}`)),
  sends: (before?: string) => data<{ sends: Send[]; next_before: string | null }>(api.get(`${base}/sends`, { params: before ? { before } : {} })),
  due: (ruleId: string) => data<{ due: Due[] }>(api.get(`${base}/recalls/${ruleId}/due`)),
  sendRecalls: (ruleId: string, customerIds: string[]) =>
    data<{ counts: SendCounts }>(api.post(`${base}/recalls/${ruleId}/send`, { customer_ids: customerIds })),
  /** Approved templates, for the rule form (upstream's templates API). */
  templates: async (): Promise<TemplateOption[]> => {
    const r = await templatesService.list({ status: 'APPROVED', limit: 200 })
    const list = ((r.data as unknown as { data?: { templates?: TemplateOption[] } }).data?.templates ?? []) as TemplateOption[]
    return list.map(t => ({ name: t.name, language: t.language }))
  }
}
