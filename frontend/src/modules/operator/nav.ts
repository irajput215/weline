import { Building2 } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

// Only super admins pass this permission check (no role is given it).
export const nav: NavSection[] = [
  {
    label: 'wl.operator.section',
    permissions: ['wl_operator_console'],
    items: [{ name: 'wl.operator.nav.console', path: '/x/operator', icon: Building2, permission: 'wl_operator_console' }]
  }
]
