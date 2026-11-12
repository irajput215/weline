import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const c = vi.hoisted(() => ({
  summary: vi.fn(),
  events: vi.fn(),
  search: vi.fn(),
  contact: vi.fn(),
  record: vi.fn(),
  previewImport: vi.fn(),
  applyImport: vi.fn(),
  settings: vi.fn(),
  saveSettings: vi.fn()
}))
vi.mock('../api', () => ({ consentApi: c }))

import { normaliseKeyword, useConsentStore } from './consent'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })

describe('consent store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(c).forEach(fn => fn.mockReset())
  })

  it('loads counts and history, null as empty', async () => {
    c.summary.mockResolvedValue({ opted_in: 1, opted_out: 2, not_recorded: 3 })
    c.events.mockResolvedValue({ events: null, next_before: null })
    const store = useConsentStore()
    await store.loadOverview()
    expect(store.state.overview).toBe('ready')
    expect(store.summary?.opted_out).toBe(2)
    expect(store.events).toEqual([])
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useConsentStore()
    c.summary.mockRejectedValueOnce(httpError(403))
    c.events.mockResolvedValue({ events: [] })
    await store.loadOverview()
    expect(store.state.overview).toBe('forbidden')
    c.settings.mockRejectedValueOnce(httpError(500))
    await store.loadSettings()
    expect(store.state.settings).toBe('error')
  })

  it('passes filters and pages with next_before', async () => {
    c.events
      .mockResolvedValueOnce({ events: [{ id: 'a' }], next_before: '2026-11-01T00:00:00Z' })
      .mockResolvedValueOnce({ events: [{ id: 'b' }], next_before: null })
    const store = useConsentStore()
    store.filters.source = 'keyword'
    await store.applyFilters()
    expect(c.events).toHaveBeenLastCalledWith(expect.objectContaining({ source: 'keyword' }))
    await store.loadMore()
    expect(c.events).toHaveBeenLastCalledWith(expect.objectContaining({ source: 'keyword', before: '2026-11-01T00:00:00Z' }))
    expect(store.events.map(e => e.id)).toEqual(['a', 'b'])
    expect(store.nextBefore).toBeNull()
  })

  it('records the server message when saving settings fails', async () => {
    c.saveSettings.mockRejectedValueOnce(httpError(400, 'opt_in_keywords: "STOP" is also an opt-out keyword'))
    const store = useConsentStore()
    expect(await store.saveSettings({ opt_in_keywords: ['STOP'] })).toBeNull()
    expect(store.saveError).toContain('also an opt-out keyword')
    c.saveSettings.mockResolvedValueOnce({ settings: { require_opt_in: true } })
    expect((await store.saveSettings({ require_opt_in: true }))?.require_opt_in).toBe(true)
    expect(store.saveError).toBeNull()
  })
})

describe('normaliseKeyword', () => {
  it('matches the server', () => {
    expect(normaliseKeyword('  stop,  messages!! ')).toBe('STOP MESSAGES')
    expect(normaliseKeyword('opt-out')).toBe('OPT OUT')
    expect(normaliseKeyword('Yes please 🙂')).toBe('YES PLEASE')
  })
})
