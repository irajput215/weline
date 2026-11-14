import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), install: vi.fn() }))
vi.mock('../api', () => ({ packsApi: a }))

import { usePacksStore } from './packs'

describe('packs store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads packs; 403 is forbidden', async () => {
    const store = usePacksStore()
    a.list.mockResolvedValueOnce({ packs: [{ id: 'clinic', installed: null }] })
    await store.load()
    expect(store.state).toBe('ready')
    a.list.mockRejectedValueOnce({ response: { status: 403 } })
    await store.load()
    expect(store.state).toBe('forbidden')
  })

  it('installs and marks the pack installed; keeps the server message on failure', async () => {
    const store = usePacksStore()
    a.list.mockResolvedValueOnce({ packs: [{ id: 'clinic', installed: null }] })
    await store.load()
    a.install.mockResolvedValueOnce({ result: { pack_id: 'clinic', items: [{ kind: 'segment', name: 'x', status: 'created' }] } })
    expect(await store.install('clinic')).toBe(true)
    expect(store.packs[0].installed?.pack_id).toBe('clinic')
    a.install.mockRejectedValueOnce({ response: { status: 404, data: { message: 'pack not found' } } })
    expect(await store.install('nope')).toBe(false)
    expect(store.installError).toBe('pack not found')
  })
})
