import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { adsApi, type Arrival, type CampaignTotals, type EntryPoint, type EntryPointInput } from '../api'

/** What an ads page should show. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

/** Days the period picker offers; 0 means custom dates. */
export const PERIODS = [7, 30, 90] as const

export function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

export function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** A date for lists, in the browser's locale. */
export function formatDate(iso: string | null | undefined, withTime = false): string {
  if (!iso) return ''
  const opts: Intl.DateTimeFormatOptions = withTime
    ? { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }
    : { day: 'numeric', month: 'short', year: 'numeric' }
  return new Intl.DateTimeFormat(undefined, opts).format(new Date(iso))
}

/** YYYY-MM-DD in the browser's zone, days before today. */
export function daysAgo(days: number, from = new Date()): string {
  const d = new Date(from)
  d.setDate(d.getDate() - days)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/**
 * Suggests a code from a name (design.md): the first word's first three
 * letters plus the last two digits in the name, or the year's. A code already
 * taken gets a digit added.
 */
export function suggestCode(name: string, taken: string[] = [], year = new Date().getFullYear()): string {
  const words = name.toUpperCase().match(/[A-Z]+/g) ?? []
  let letters = (words[0] ?? '').slice(0, 3)
  if (letters.length < 3) letters = words.join('').slice(0, 3)
  if (!letters) return ''
  const digits = (name.match(/\d/g) ?? []).join('')
  const base = letters + (digits.length >= 2 ? digits.slice(-2) : String(year).slice(-2))
  const used = new Set(taken.map(c => c.toUpperCase()))
  if (!used.has(base)) return base
  for (let i = 2; i < 10; i++) if (!used.has(base + i)) return base + i
  return ''
}

/** True when text holds code as a whole word, the way the server matches it. */
export function hasCode(text: string, code: string): boolean {
  if (!code) return false
  return text.toUpperCase().split(/[^\p{L}\p{N}]+/u).includes(code.toUpperCase())
}

/** The wa.me link the server builds (spaces as %20). */
export function waLink(number: string, text: string): string {
  const digits = number.replace(/\D/g, '')
  return `https://wa.me/${digits}?text=${encodeURIComponent(text).replace(/[!'()*]/g, c => '%' + c.charCodeAt(0).toString(16).toUpperCase())}`
}

const NUMBER_KEY = 'wl-ads-last-number'

/** The last WhatsApp number used, kept per browser for convenience. */
export function lastNumber(): string {
  try {
    return localStorage.getItem(NUMBER_KEY) ?? ''
  } catch {
    return ''
  }
}

export function rememberNumber(n: string) {
  try {
    localStorage.setItem(NUMBER_KEY, n)
  } catch {
    // private window or storage blocked: nothing to remember
  }
}

/** Ads module: one store for its pages. */
export const useAdsStore = defineStore('wl-ads', () => {
  const listState = ref<LoadState>('idle')
  const entryState = ref<LoadState>('idle')

  const days = ref<number>(30)
  const range = reactive({ from: '', to: '' })
  const showArchived = ref(false)

  const entrypoints = ref<EntryPoint[]>([])
  const campaigns = ref<CampaignTotals[]>([])

  const entry = ref<EntryPoint | null>(null)
  const arrivals = ref<Arrival[]>([])
  const nextBefore = ref<string | null>(null)
  const loadingMore = ref(false)

  const saving = ref(false)
  /** Message from the last failed save (validation, conflict or permission). */
  const saveError = ref<string | null>(null)
  /** HTTP status of the last failed save, to place the message (409 = code). */
  const saveStatus = ref<number | null>(null)

  function period() {
    if (days.value === 0) return { from: range.from || undefined, to: range.to || undefined }
    return { from: daysAgo(days.value - 1), to: daysAgo(0) }
  }

  async function load(state: typeof listState, fn: () => Promise<void>) {
    state.value = 'loading'
    try {
      await fn()
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  const loadList = () => load(listState, async () => {
    const l = await adsApi.list({ ...period(), include_archived: showArchived.value })
    entrypoints.value = l.entrypoints ?? []
    campaigns.value = l.campaigns ?? []
  })

  /** Reloads the list in place (period or archived filter changed). */
  async function refreshList() {
    try {
      const l = await adsApi.list({ ...period(), include_archived: showArchived.value })
      entrypoints.value = l.entrypoints ?? []
      campaigns.value = l.campaigns ?? []
    } catch (err) {
      listState.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  const loadEntry = (id: string) => load(entryState, async () => {
    const [e, a] = await Promise.all([adsApi.get(id, period()), adsApi.arrivals(id)])
    entry.value = e.entrypoint
    arrivals.value = a.arrivals ?? []
    nextBefore.value = a.next_before
  })

  async function loadMore() {
    if (!entry.value || !nextBefore.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const a = await adsApi.arrivals(entry.value.id, nextBefore.value)
      arrivals.value = [...arrivals.value, ...(a.arrivals ?? [])]
      nextBefore.value = a.next_before
    } finally {
      loadingMore.value = false
    }
  }

  /** Runs a save; returns its result, or null after recording the error. */
  async function save<T>(fn: () => Promise<T>): Promise<T | null> {
    saving.value = true
    saveError.value = null
    saveStatus.value = null
    try {
      return await fn()
    } catch (err) {
      saveStatus.value = statusOf(err) ?? null
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return null
    } finally {
      saving.value = false
    }
  }

  async function create(body: EntryPointInput) {
    const r = await save(() => adsApi.create(body))
    if (r) rememberNumber(body.wa_number)
    return r?.entrypoint ?? null
  }

  async function update(id: string, body: EntryPointInput) {
    const r = await save(() => adsApi.update(id, body))
    if (r) entry.value = { ...r.entrypoint, stats: entry.value?.stats ?? r.entrypoint.stats }
    return r?.entrypoint ?? null
  }

  async function archive(id: string) {
    const r = await save(() => adsApi.archive(id))
    if (r && entry.value?.id === id) entry.value = { ...r.entrypoint, stats: entry.value.stats }
    return r?.entrypoint ?? null
  }

  async function remove(id: string) {
    const r = await save(() => adsApi.remove(id))
    return !!r?.deleted
  }

  return {
    listState, entryState, days, range, showArchived, entrypoints, campaigns, entry, arrivals, nextBefore, loadingMore,
    saving, saveError, saveStatus,
    period, loadList, refreshList, loadEntry, loadMore, create, update, archive, remove
  }
})
