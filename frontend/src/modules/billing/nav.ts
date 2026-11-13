import { CreditCard } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.billing.section',
    permissions: ['wl_billing_plan'],
    items: [{ name: 'wl.billing.nav.billing', path: '/x/billing', icon: CreditCard, permission: 'wl_billing_plan' }]
  }
]
