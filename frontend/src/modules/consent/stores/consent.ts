import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { consentApi, type ConsentEvent, type EventQuery, type Settings, type Summary } from '../api'

/** What a consent page should show for one resource. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Resource = 'overview' | 'settings'

export const SOURCES = ['keyword', 'button', 'staff', 'import', 'meta'] as const

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

export function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** Upper case, anything but letters and digits as a space: the server's normalising (design.md). */
export function normaliseKeyword(s: string): string {
  return s
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim()
    .toUpperCase()
}

/** A date for lists, in the browser's locale. */
export function formatDate(iso: string | null | undefined, withTime = false): string {
  if (!iso) return ''
  const opts: Intl.DateTimeFormatOptions = withTime
    ? { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }
    : { day: 'numeric', month: 'short', year: 'numeric' }
  return new Intl.DateTimeFormat(undefined, opts).format(new Date(iso))
}

/** Today in the browser's zone, as YYYY-MM-DD. */
export function today(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Consent module: one store for its pages. */
export const useConsentStore = defineStore('wl-consent', () => {
  const state = reactive<Record<Resource, LoadState>>({ overview: 'idle', settings: 'idle' })

  const summary = ref<Summary | null>(null)
  const events = ref<ConsentEvent[]>([])
  const nextBefore = ref<string | null>(null)
  const filters = reactive<EventQuery>({ action: '', source: '', from: '', to: '' })
  const loadingMore = ref(false)

  const settings = ref<Settings | null>(null)
  const defaults = ref<Settings | null>(null)

  const saving = ref(false)
  /** Message from the last failed save (validation, conflict or permission). */
  const saveError = ref<string | null>(null)

  async function load(resource: Resource, fn: () => Promise<void>) {
    state[resource] = 'loading'
    try {
      await fn()
      state[resource] = 'ready'
    } catch (err) {
      state[resource] = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  /** Runs a save; returns its result, or null after recording the error. */
  async function save<T>(fn: () => Promise<T>): Promise<T | null> {
    saving.value = true
    saveError.value = null
    try {
      return await fn()
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return null
    } finally {
      saving.value = false
    }
  }

  const loadOverview = () => load('overview', async () => {
    const [s, e] = await Promise.all([consentApi.summary(), consentApi.events({ ...filters })])
    summary.value = s
    events.value = e.events ?? []
    nextBefore.value = e.next_before
  })

  /**
   * Updates counts and history in place, without the loading state, so dialogs
   * open on the page stay as they are.
   */
  async function refresh() {
    try {
      const [s, e] = await Promise.all([consentApi.summary(), consentApi.events({ ...filters })])
      summary.value = s
      events.value = e.events ?? []
      nextBefore.value = e.next_before
    } catch {
      // keep what's shown; the next load reports the problem
    }
  }

  /** Reloads the history with the current filters (the counts don't change). */
  async function applyFilters() {
    try {
      const e = await consentApi.events({ ...filters })
      events.value = e.events ?? []
      nextBefore.value = e.next_before
    } catch (err) {
      state.overview = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function loadMore() {
    if (!nextBefore.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const e = await consentApi.events({ ...filters, before: nextBefore.value })
      events.value = [...events.value, ...(e.events ?? [])]
      nextBefore.value = e.next_before
    } finally {
      loadingMore.value = false
    }
  }

  const loadSettings = () => load('settings', async () => {
    const r = await consentApi.settings()
    settings.value = r.settings
    defaults.value = r.defaults
  })

  async function saveSettings(body: Partial<Settings>) {
    const r = await save(() => consentApi.saveSettings(body))
    if (r) settings.value = r.settings
    return r?.settings ?? null
  }

  return {
    state, summary, events, nextBefore, filters, loadingMore, settings, defaults, saving, saveError,
    loadOverview, refresh, applyFilters, loadMore, loadSettings, saveSettings, save
  }
})
