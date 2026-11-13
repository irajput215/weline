import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const c = vi.hoisted(() => ({ account: vi.fn(), save: vi.fn(), test: vi.fn(), syncs: vi.fn() }))
vi.mock('../api', () => ({ connectorsApi: c }))

import { useConnectorsStore } from './connectors'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })
const builtin = { provider: 'builtin', status: 'connected', credentials_set: false }

describe('connectors store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(c).forEach(fn => fn.mockReset())
    c.syncs.mockResolvedValue({ syncs: [] })
  })

  it('loads the account, providers and log; null as empty', async () => {
    c.account.mockResolvedValue({ account: builtin, providers: null })
    c.syncs.mockResolvedValue({ syncs: null })
    const store = useConnectorsStore()
    await store.load()
    expect(store.state).toBe('ready')
    expect(store.account?.provider).toBe('builtin')
    expect(store.providers).toEqual([])
    expect(store.syncs).toEqual([])
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useConnectorsStore()
    c.account.mockRejectedValueOnce(httpError(403))
    await store.load()
    expect(store.state).toBe('forbidden')
    c.account.mockRejectedValueOnce(httpError(500))
    await store.load()
    expect(store.state).toBe('error')
  })

  it('saves with credentials only when given, and keeps the server message on failure', async () => {
    const store = useConnectorsStore()
    c.save.mockRejectedValueOnce(httpError(400, 'connection test failed: the API key was refused'))
    expect(await store.save('cliniko', { api_key: 'x' })).toBe(false)
    expect(store.saveError).toContain('refused')
    expect(c.save).toHaveBeenLastCalledWith({ provider: 'cliniko', credentials: { api_key: 'x' } })

    c.save.mockResolvedValueOnce({ account: { ...builtin, provider: 'cliniko' } })
    expect(await store.save('cliniko')).toBe(true)
    expect(c.save).toHaveBeenLastCalledWith({ provider: 'cliniko', credentials: undefined })
    expect(store.account?.provider).toBe('cliniko')
    expect(store.saveError).toBeNull()
    expect(c.syncs).toHaveBeenCalled()
  })

  it('records a test result and a forbidden test', async () => {
    const store = useConnectorsStore()
    c.test.mockResolvedValueOnce({ ok: false, message: 'the API key was refused', account: { ...builtin, status: 'error' } })
    await store.test()
    expect(store.testMessage).toEqual({ ok: false, message: 'the API key was refused' })
    expect(store.account?.status).toBe('error')
    c.test.mockRejectedValueOnce(httpError(403))
    await store.test()
    expect(store.saveError).toBe('forbidden')
  })
})
