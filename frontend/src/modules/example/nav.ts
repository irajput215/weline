import { StickyNote } from 'lucide-vue-next'
import type { NavSection } from '@/components/layout/navigation'

export const nav: NavSection[] = [
  {
    label: 'wl.example.section',
    permissions: ['wl_example_notes'],
    items: [
      {
        name: 'wl.example.navItem',
        path: '/x/example/notes',
        icon: StickyNote,
        permission: 'wl_example_notes'
      }
    ]
  }
]
