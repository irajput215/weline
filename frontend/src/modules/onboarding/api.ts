// Calls to /api/x/onboarding. Spec: wl-docs/modules/onboarding/api.md
import { api } from '@/services/api'

export type StepKey = 'business' | 'whatsapp' | 'system' | 'vertical' | 'team' | 'first_message'
export const verticals = ['allied_health', 'dental', 'medical', 'beauty', 'fitness', 'real_estate', 'other'] as const
export type Vertical = (typeof verticals)[number]

export interface Step {
  key: StepKey
  done: boolean
  link: string
}

export interface Checklist {
  vertical: Vertical | ''
  dismissed: boolean
  steps: Step[]
  done: number
  total: number
}

const base = '/x/onboarding'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)

export const onboardingApi = {
  checklist: () => data<Checklist>(api.get(`${base}/checklist`)),
  setVertical: (vertical: Vertical) => data<Checklist>(api.put(`${base}/vertical`, { vertical })),
  setDismissed: (dismissed: boolean) => data<Checklist>(api.put(`${base}/dismissed`, { dismissed }))
}
