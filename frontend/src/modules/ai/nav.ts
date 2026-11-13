import { Bot } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.ai.section',
    permissions: ['wl_ai_runs', 'wl_ai_settings'],
    items: [{ name: 'wl.ai.nav.agent', path: '/x/ai', icon: Bot, permission: 'wl_ai_runs' }]
  }
]
