// Calls to /api/x/audiences through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it). Spec:
// wl-docs/modules/audiences/api.md
import { api } from '@/services/api'

export type Field = 'tag' | 'metadata' | 'whatsapp_account' | 'last_inbound' | 'created' | 'campaign' | 'consent'

export interface Condition {
  field: Field
  op: string
  key?: string
  value?: string | number
  days?: number
}

export interface Rules {
  match: 'all' | 'any'
  conditions: Condition[]
}

export interface Segment {
  id: string
  name: string
  description: string
  rules: Rules
  count?: number
  updated_at: string
  last_used_at: string | null
}

export interface Excluded {
  opted_out: number
  no_opt_in: number
  no_number: number
  duplicates: number
  over_limit: number
}

export interface Preview {
  matched: number
  recipients: number
  excluded: Excluded
  marketing_opt_out: number
  not_recorded: number
  require_opt_in: boolean
  sample: { name: string; phone_masked: string }[]
}

export type RetargetFilter = 'not_received' | 'not_read' | 'read_no_reply' | 'replied'

export interface Retarget {
  campaign_id: string
  filter: RetargetFilter
  days?: number
}

export interface CampaignOption {
  id: string
  name: string
  status: string
  total: number
  sent: number
  delivered: number
  read: number
  failed: number
  completed_at: string | null
}

export interface TemplateOption {
  id: string
  name: string
  language: string
  category: string
  whatsapp_account: string
  header_type: string
  header_content: string
  body_content: string
  body_vars: string[]
  header_vars: string[]
}

export interface Fields {
  tags: string[]
  metadata_keys: string[]
  accounts: string[]
  campaigns: CampaignOption[]
  templates: TemplateOption[]
}

export type VarSourceKind = 'text' | 'name' | 'phone' | 'metadata'

export interface VarSource {
  source: VarSourceKind
  value?: string
  key?: string
  fallback?: string
}

export interface SendInput {
  name: string
  whatsapp_account: string
  template_id: string
  segment_id?: string
  retarget?: Retarget
  variables: { body: Record<string, VarSource>; header: Record<string, VarSource> }
  confirm_count: number
}

export interface SendResult {
  campaign_id: string
  recipients: number
  excluded: Excluded
}

export interface Send {
  id: string
  kind: 'segment' | 'retarget'
  segment_id: string | null
  source_campaign_id: string | null
  retarget_filter: RetargetFilter | null
  campaign_id: string
  campaign_name: string
  recipient_count: number
  excluded: Partial<Excluded>
  created_by: { id: string; name: string } | null
  created_at: string
}

export interface PreviewInput {
  rules?: Rules
  retarget?: Retarget
  template_id?: string
}

const base = '/x/audiences'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const audiencesApi = {
  listSegments: () => data<{ segments: Segment[] }>(api.get(`${base}/segments`)),
  getSegment: (id: string) => data<{ segment: Segment }>(api.get(`${base}/segments/${id}`)),
  createSegment: (body: { name: string; description: string; rules: Rules }) =>
    data<{ segment: Segment }>(api.post(`${base}/segments`, body)),
  updateSegment: (id: string, body: Partial<{ name: string; description: string; rules: Rules }>) =>
    data<{ segment: Segment }>(api.put(`${base}/segments/${id}`, body)),
  deleteSegment: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/segments/${id}`)),
  preview: (body: PreviewInput) => data<Preview>(api.post(`${base}/preview`, body)),
  fields: () => data<Fields>(api.get(`${base}/fields`)),
  createSend: (body: SendInput) => data<SendResult>(api.post(`${base}/sends`, body)),
  listSends: (q: { segment_id?: string; campaign_id?: string } = {}) =>
    data<{ sends: Send[] }>(api.get(`${base}/sends`, { params: q }))
}
