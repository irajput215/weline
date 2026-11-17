import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ search: vi.fn(), get: vi.fn() }))
vi.mock('../api', () => ({ timelineApi: a }))

import { useTimelineStore } from './timeline'

describe('timeline store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('searches only from 2 characters', async () => {
    const store = useTimelineStore()
    await store.search('p')
    expect(a.search).not.toHaveBeenCalled()
    a.search.mockResolvedValueOnce({ contacts: [{ id: 'c1', name: 'Pat' }] })
    await store.search('pat')
    expect(store.results).toHaveLength(1)
  })

  it('opens a timeline, pages, and maps 403 and 404', async () => {
    const store = useTimelineStore()
    a.get.mockResolvedValueOnce({ contact: { id: 'c1' }, items: [{ kind: 'message_in', at: '2027-01-02T00:00:00Z', fields: {} }], next_before: '2027-01-01T00:00:00Z' })
    await store.open('c1')
    expect(store.state).toBe('ready')
    a.get.mockResolvedValueOnce({ contact: { id: 'c1' }, items: [{ kind: 'consent', at: '2026-12-01T00:00:00Z', fields: {} }], next_before: null })
    await store.more()
    expect(a.get).toHaveBeenLastCalledWith('c1', '2027-01-01T00:00:00Z')
    expect(store.timeline?.items).toHaveLength(2)
    expect(store.timeline?.next_before).toBeNull()
    a.get.mockRejectedValueOnce({ response: { status: 404 } })
    await store.open('x')
    expect(store.state).toBe('notfound')
    a.get.mockRejectedValueOnce({ response: { status: 403 } })
    await store.open('x')
    expect(store.state).toBe('forbidden')
  })
})
