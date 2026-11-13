import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ checklist: vi.fn(), setVertical: vi.fn(), setDismissed: vi.fn() }))
vi.mock('../api', () => ({ onboardingApi: a }))

import { useOnboardingStore } from './onboarding'

const list = { vertical: '', dismissed: false, steps: [{ key: 'business', done: true, link: '/x/practice/profile' }], done: 1, total: 6 }

describe('onboarding store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads the checklist; 403 is forbidden, other failures an error', async () => {
    const store = useOnboardingStore()
    a.checklist.mockResolvedValueOnce(list)
    await store.load()
    expect(store.state).toBe('ready')
    expect(store.checklist?.done).toBe(1)
    a.checklist.mockRejectedValueOnce({ response: { status: 403 } })
    await store.load()
    expect(store.state).toBe('forbidden')
    a.checklist.mockRejectedValueOnce(new Error('offline'))
    await store.load()
    expect(store.state).toBe('error')
  })

  it('saves the vertical and keeps the server message on failure', async () => {
    const store = useOnboardingStore()
    a.setVertical.mockResolvedValueOnce({ ...list, vertical: 'dental' })
    expect(await store.setVertical('dental')).toBe(true)
    expect(store.checklist?.vertical).toBe('dental')
    a.setDismissed.mockRejectedValueOnce({ response: { status: 403 } })
    expect(await store.setDismissed(true)).toBe(false)
    expect(store.saveError).toBe('forbidden')
  })
})
