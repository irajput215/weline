import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  archive: vi.fn(),
  remove: vi.fn(),
  arrivals: vi.fn()
}))
vi.mock('../api', () => ({ adsApi: a, CHANNELS: ['meta_ad'] }))

// An in-memory localStorage: the unit test environment has none.
const memory = new Map<string, string>()
vi.stubGlobal('localStorage', {
  getItem: (k: string) => memory.get(k) ?? null,
  setItem: (k: string, v: string) => void memory.set(k, v),
  clear: () => memory.clear()
})

import type { EntryPoint } from '../api'
import { daysAgo, hasCode, suggestCode, useAdsStore, waLink } from './ads'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })
const body = { name: 'Flu', code: 'FLU24', channel: 'meta_ad' as const, campaign: '', wa_number: '61400000000',
  prefill_text: 'Hi [FLU24]', tag: '', tag_campaign: true }

describe('ads helpers', () => {
  it('suggests a code from the name', () => {
    expect(suggestCode('Flu shot ad 2024')).toBe('FLU24')
    expect(suggestCode('Skin check', [], 2026)).toBe('SKI26')
    expect(suggestCode('A b c 7 8', [], 2026)).toBe('ABC78')
    expect(suggestCode('Flu shot 2024', ['flu24'])).toBe('FLU242')
    expect(suggestCode('2024')).toBe('')
  })

  it('finds the code as a whole word, like the server', () => {
    expect(hasCode('Hi! [FLU24]', 'flu24')).toBe(true)
    expect(hasCode('#flu24 please', 'FLU24')).toBe(true)
    expect(hasCode('XFLU24', 'FLU24')).toBe(false)
    expect(hasCode('FLU245', 'FLU24')).toBe(false)
  })

  it('builds the same link as the server', () => {
    expect(waLink('+61 400 000 000', "Hi! I'd like to know more [FLU24]")).toBe(
      'https://wa.me/61400000000?text=Hi%21%20I%27d%20like%20to%20know%20more%20%5BFLU24%5D'
    )
  })

  it('works out dates for the period', () => {
    expect(daysAgo(0, new Date(2026, 10, 13))).toBe('2026-11-13')
    expect(daysAgo(29, new Date(2026, 10, 13))).toBe('2026-10-15')
  })
})

describe('ads store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
    localStorage.clear()
  })

  it('loads the list, null as empty, for the last 30 days', async () => {
    a.list.mockResolvedValue({ entrypoints: null, campaigns: null })
    const store = useAdsStore()
    await store.loadList()
    expect(store.listState).toBe('ready')
    expect(store.entrypoints).toEqual([])
    expect(store.campaigns).toEqual([])
    const q = a.list.mock.calls[0][0]
    expect(q.to).toBe(daysAgo(0))
    expect(q.from).toBe(daysAgo(29))
    expect(q.include_archived).toBe(false)
  })

  it('uses custom dates and the archived filter', async () => {
    a.list.mockResolvedValue({ entrypoints: [], campaigns: [] })
    const store = useAdsStore()
    store.days = 0
    store.range.from = '2026-01-01'
    store.range.to = '2026-01-31'
    store.showArchived = true
    await store.refreshList()
    expect(a.list).toHaveBeenCalledWith({ from: '2026-01-01', to: '2026-01-31', include_archived: true })
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useAdsStore()
    a.list.mockRejectedValueOnce(httpError(403))
    await store.loadList()
    expect(store.listState).toBe('forbidden')
    a.get.mockRejectedValueOnce(httpError(500))
    a.arrivals.mockResolvedValue({ arrivals: [], next_before: null })
    await store.loadEntry('x')
    expect(store.entryState).toBe('error')
  })

  it('loads an entry point with its arrivals and pages with next_before', async () => {
    a.get.mockResolvedValue({ entrypoint: { id: 'e1', stats: { conversations: 1 } } })
    a.arrivals
      .mockResolvedValueOnce({ arrivals: [{ id: 'a1' }], next_before: '2026-11-01T00:00:00Z' })
      .mockResolvedValueOnce({ arrivals: [{ id: 'a2' }], next_before: null })
    const store = useAdsStore()
    await store.loadEntry('e1')
    expect(store.entryState).toBe('ready')
    await store.loadMore()
    expect(a.arrivals).toHaveBeenLastCalledWith('e1', '2026-11-01T00:00:00Z')
    expect(store.arrivals.map(x => x.id)).toEqual(['a1', 'a2'])
    expect(store.nextBefore).toBeNull()
  })

  it('keeps a 409 on the code and remembers the number after a save', async () => {
    const store = useAdsStore()
    a.create.mockRejectedValueOnce(httpError(409, 'code: FLU24 is already used'))
    expect(await store.create(body)).toBeNull()
    expect(store.saveStatus).toBe(409)
    expect(store.saveError).toContain('FLU24')
    expect(localStorage.getItem('wl-ads-last-number')).toBeNull()

    a.create.mockResolvedValueOnce({ entrypoint: { id: 'e1' } })
    expect(await store.create(body)).toEqual({ id: 'e1' })
    expect(store.saveError).toBeNull()
    expect(localStorage.getItem('wl-ads-last-number')).toBe('61400000000')
  })

  it('keeps the counts when editing or archiving, and reports a forbidden save', async () => {
    const store = useAdsStore()
    store.entry = { id: 'e1', stats: { conversations: 5 } } as unknown as EntryPoint
    a.update.mockResolvedValueOnce({ entrypoint: { id: 'e1', name: 'New', stats: { conversations: 0 } } })
    await store.update('e1', body)
    expect(store.entry?.name).toBe('New')
    expect(store.entry?.stats.conversations).toBe(5)
    a.archive.mockResolvedValueOnce({ entrypoint: { id: 'e1', archived_at: 'now', stats: { conversations: 0 } } })
    await store.archive('e1')
    expect(store.entry?.archived_at).toBe('now')
    expect(store.entry?.stats.conversations).toBe(5)
    a.remove.mockRejectedValueOnce(httpError(403))
    expect(await store.remove('e1')).toBe(false)
    expect(store.saveError).toBe('forbidden')
  })
})
