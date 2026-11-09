import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import { LayoutDashboard } from 'lucide-vue-next'
import {
  checkConventions,
  collectMessages,
  collectNav,
  collectRoutes,
  mergeModuleMessages,
  moduleConventionProblems,
  moduleNav,
  moduleRoutes
} from './index'

const page = () => Promise.resolve({ default: {} })

describe('module index', () => {
  it('modules in the repo follow the conventions', () => {
    // Fails as soon as a module adds a route or nav item without a wl_<module>_ permission.
    expect(moduleConventionProblems).toEqual([])
  })

  it('is empty while no module exists', () => {
    expect(moduleRoutes).toEqual([])
    expect(moduleNav).toEqual([])
  })

  it('collects routes and nav in module-name order', () => {
    const routes = collectRoutes({
      './notes/routes.ts': { routes: [{ path: 'notes', component: page, meta: { permission: 'wl_notes_items' } }] },
      './agenda/routes.ts': { routes: [{ path: 'agenda', component: page, meta: { permission: 'wl_agenda_items' } }] },
      './empty/routes.ts': {}
    })
    expect(routes.map(r => r.path)).toEqual(['agenda', 'notes'])

    const nav = collectNav({
      './notes/nav.ts': { nav: [{ label: 'wl.notes.section', permissions: [], items: [] }] },
      './agenda/nav.ts': { nav: [{ label: 'wl.agenda.section', permissions: [], items: [] }] }
    })
    expect(nav.map(s => s.label)).toEqual(['wl.agenda.section', 'wl.notes.section'])
  })

  it('namespaces strings under wl.<module> per locale', () => {
    expect(
      collectMessages({
        './booking/i18n/en.json': { title: 'Bookings' },
        './booking/i18n/es.json': { title: 'Reservas' },
        './practice/i18n/en.json': { title: 'Practice' }
      })
    ).toEqual({
      en: { wl: { booking: { title: 'Bookings' }, practice: { title: 'Practice' } } },
      es: { wl: { booking: { title: 'Reservas' } } }
    })
  })

  it('merges module strings without touching upstream keys', () => {
    const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: { nav: { chat: 'Chat' } } } })
    mergeModuleMessages(i18n)
    expect(i18n.global.t('nav.chat')).toBe('Chat')
  })

  it('reports routes and nav items without a wl_<module>_ permission', () => {
    const problems = checkConventions(
      {
        './notes/routes.ts': {
          routes: [
            { path: 'ok', name: 'ok', component: page, meta: { permission: 'wl_notes_items' } },
            { path: 'open', name: 'open', component: page },
            { path: 'stolen', name: 'stolen', component: page, meta: { permission: 'wl_billing_items' } }
          ]
        }
      },
      {
        './notes/nav.ts': {
          nav: [
            {
              label: 'wl.notes.section',
              permissions: [],
              items: [
                { name: 'wl.notes.title', path: '/notes', icon: LayoutDashboard, permission: 'wl_notes_items' },
                { name: 'wl.notes.unguarded', path: '/notes/x', icon: LayoutDashboard }
              ]
            }
          ]
        }
      }
    )
    expect(problems).toHaveLength(3)
    expect(problems.join('\n')).toMatch(/'open'/)
    expect(problems.join('\n')).toMatch(/'stolen'/)
    expect(problems.join('\n')).toMatch(/'wl.notes.unguarded'/)
  })
})
