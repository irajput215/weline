// Calls to /api/x/ai. Spec: wl-docs/modules/ai/api.md
import { api } from '@/services/api'

export interface AiSettings {
  enabled: boolean
  handover_team_id: string | null
  tone: 'friendly' | 'formal' | 'brief'
  instructions: string
  confidence_threshold: number
  emergency_words: string[]
}

export interface ProviderInfo {
  name: string
  model: string
  key_set: boolean
}

export interface KnowledgeItem {
  source_id: string
  kind: 'faq' | 'document'
  title: string
  chunks: number
  created_at: string
}

export interface Run {
  id: string
  contact: { id: string; name: string; phone_masked: string }
  intent: string
  confidence: number
  outcome: string
  latency_ms: number
  created_at: string
}

const base = '/x/ai'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const aiApi = {
  settings: () => data<{ settings: AiSettings; provider: ProviderInfo }>(api.get(`${base}/settings`)),
  saveSettings: (body: AiSettings) => data<{ settings: AiSettings }>(api.put(`${base}/settings`, body)),
  knowledge: () => data<{ items: KnowledgeItem[] }>(api.get(`${base}/knowledge`)),
  addKnowledge: (body: { kind: 'faq'; question: string; answer: string } | { kind: 'document'; title: string; text: string }) =>
    data<{ item: KnowledgeItem }>(api.post(`${base}/knowledge`, body)),
  deleteKnowledge: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/knowledge/${id}`)),
  runs: (before?: string) => data<{ runs: Run[]; next_before: string | null }>(api.get(`${base}/runs`, { params: before ? { before } : {} }))
}
