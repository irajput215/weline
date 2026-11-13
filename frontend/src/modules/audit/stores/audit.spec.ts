import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ events: vi.fn() }))
vi.mock('../api', () => ({ auditApi: a }))

import { useAuditStore } from './audit'

describe('audit store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    a.events.mockReset()
  })

  it('loads events with the area filter and pages with next_before', async () => {
    a.events
      .mockResolvedValueOnce({ events: [{ id: '1' }], next_before: '2026-11-01T00:00:00Z' })
      .mockResolvedValueOnce({ events: null, next_before: null })
    const store = useAuditStore()
    store.module = 'connectors'
    await store.load()
    expect(store.state).toBe('ready')
    expect(a.events).toHaveBeenLastCalledWith({ module: 'connectors' })
    await store.loadMore()
    expect(a.events).toHaveBeenLastCalledWith({ module: 'connectors', before: '2026-11-01T00:00:00Z' })
    expect(store.events.map(e => e.id)).toEqual(['1'])
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useAuditStore()
    a.events.mockRejectedValueOnce({ response: { status: 403 } })
    await store.load()
    expect(store.state).toBe('forbidden')
    a.events.mockRejectedValueOnce({ response: { status: 500 } })
    await store.load()
    expect(store.state).toBe('error')
  })
})
