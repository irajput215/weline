import { Package } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.packs.section',
    permissions: ['wl_packs_install'],
    items: [{ name: 'wl.packs.nav.packs', path: '/x/packs', icon: Package, permission: 'wl_packs_install' }]
  }
]
