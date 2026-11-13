import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ overview: vi.fn(), tenants: vi.fn(), tenant: vi.fn() }))
vi.mock('../api', () => ({ operatorApi: a }))

import { useOperatorStore } from './operator'

describe('operator store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads overview and organizations; 403 (not an operator) is forbidden', async () => {
    const store = useOperatorStore()
    a.overview.mockResolvedValueOnce({ organizations: 2 })
    a.tenants.mockResolvedValueOnce({ tenants: [{ id: 'o1' }], next_offset: 50 })
    await store.load()
    expect(store.state).toBe('ready')
    expect(store.nextOffset).toBe(50)
    a.tenants.mockResolvedValueOnce({ tenants: [{ id: 'o2' }], next_offset: null })
    await store.loadMore()
    expect(store.tenants.map(t => t.id)).toEqual(['o1', 'o2'])
    a.overview.mockRejectedValueOnce({ response: { status: 403 } })
    a.tenants.mockResolvedValueOnce({ tenants: [], next_offset: null })
    await store.load()
    expect(store.state).toBe('forbidden')
  })

  it('searches and opens one organization', async () => {
    const store = useOperatorStore()
    a.tenants.mockResolvedValueOnce({ tenants: [], next_offset: null })
    await store.find('physio')
    expect(a.tenants).toHaveBeenCalledWith('physio')
    a.tenant.mockRejectedValueOnce(new Error('offline'))
    await store.open('o1')
    expect(store.detailState).toBe('error')
    a.tenant.mockResolvedValueOnce({ id: 'o1', health: {} })
    await store.open('o1')
    expect(store.detail?.id).toBe('o1')
  })
})
