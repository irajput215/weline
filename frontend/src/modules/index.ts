// Weline module index. Collects every module's routes, sidebar sections and
// strings so upstream files only need one seam line each:
//
//   router/index.ts                  ...moduleRoutes,          // Weline hook
//   components/layout/AppLayout.vue  .concat(moduleNav)        // Weline hook
//   i18n/index.ts                    mergeModuleMessages(i18n) // Weline hook
//
// A module lives in frontend/src/modules/<module>/ and is picked up from its
// folder, no registration needed:
//   routes.ts       export const routes: RouteRecordRaw[]   (children of the app layout)
//   nav.ts          export const nav: NavSection[]          (sidebar sections)
//   i18n/<lang>.json  the module's strings, available as wl.<module>.<key>
//
// Weline-only file.
import type { RouteRecordRaw } from 'vue-router'
import type { I18n } from 'vue-i18n'
import type { NavSection } from '@/components/layout/navigation'

type RoutesFile = { routes?: RouteRecordRaw[] }
type NavFile = { nav?: NavSection[] }
type Messages = Record<string, unknown>

// Folder name of a module file, e.g. './booking/routes.ts' -> 'booking'.
function moduleOf(path: string): string {
  return path.split('/')[1]
}

function sortedEntries<T>(files: Record<string, T>): [string, T][] {
  return Object.entries(files).sort(([a], [b]) => a.localeCompare(b))
}

/** Every route a module adds, in module-name order. */
export function collectRoutes(files: Record<string, RoutesFile>): RouteRecordRaw[] {
  return sortedEntries(files).flatMap(([, f]) => f.routes ?? [])
}

/** Every sidebar section a module adds, in module-name order. */
export function collectNav(files: Record<string, NavFile>): NavSection[] {
  return sortedEntries(files).flatMap(([, f]) => f.nav ?? [])
}

/**
 * Groups module strings by locale under wl.<module>, so a module can't
 * overwrite upstream keys or another module's keys.
 * './booking/i18n/en.json' with { "title": "Bookings" } -> en: { wl: { booking: { title } } }
 */
export function collectMessages(files: Record<string, Messages>): Record<string, { wl: Record<string, Messages> }> {
  const byLocale: Record<string, { wl: Record<string, Messages> }> = {}
  for (const [path, messages] of sortedEntries(files)) {
    const locale = path.split('/').pop()!.replace('.json', '')
    byLocale[locale] ??= { wl: {} }
    byLocale[locale].wl[moduleOf(path)] = messages
  }
  return byLocale
}

/**
 * Checks the conventions the backend relies on: every module route is
 * permission-checked with a wl_<module>_ permission, and every nav item that
 * links to a module page carries one too. Returns the problems found.
 */
export function checkConventions(routeFiles: Record<string, RoutesFile>, navFiles: Record<string, NavFile>): string[] {
  const problems: string[] = []
  for (const [path, f] of Object.entries(routeFiles)) {
    const prefix = `wl_${moduleOf(path)}_`
    for (const r of f.routes ?? []) {
      const perm = r.meta?.permission
      if (!perm || !perm.startsWith(prefix)) {
        problems.push(`${path}: route '${String(r.name ?? r.path)}' needs meta.permission starting with '${prefix}'`)
      }
    }
  }
  for (const [path, f] of Object.entries(navFiles)) {
    const prefix = `wl_${moduleOf(path)}_`
    for (const section of f.nav ?? []) {
      for (const item of section.items) {
        if (!item.permission || !item.permission.startsWith(prefix)) {
          problems.push(`${path}: nav item '${item.name}' needs a permission starting with '${prefix}'`)
        }
      }
    }
  }
  return problems
}

const routeFiles = import.meta.glob<RoutesFile>('./*/routes.ts', { eager: true })
const navFiles = import.meta.glob<NavFile>('./*/nav.ts', { eager: true })
const messageFiles = import.meta.glob<Messages>('./*/i18n/*.json', { eager: true, import: 'default' })

/** Module pages, spread into the authenticated app layout's children. */
export const moduleRoutes: RouteRecordRaw[] = collectRoutes(routeFiles)

/** Module sidebar sections, appended after upstream's sections. */
export const moduleNav: NavSection[] = collectNav(navFiles)

/** The part of a vue-i18n instance mergeModuleMessages needs (any message schema). */
type MergeTarget = { global: Pick<I18n<{}, {}, {}, string, false>['global'], 'mergeLocaleMessage'> }

/** Adds every module's strings to the app's i18n instance. */
export function mergeModuleMessages(i18n: MergeTarget): void {
  for (const [locale, messages] of Object.entries(collectMessages(messageFiles))) {
    i18n.global.mergeLocaleMessage(locale, messages)
  }
}

/** Convention problems in the modules actually present (checked by a unit test). */
export const moduleConventionProblems: string[] = checkConventions(routeFiles, navFiles)
