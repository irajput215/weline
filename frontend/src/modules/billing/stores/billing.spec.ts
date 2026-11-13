import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ summary: vi.fn(), invoices: vi.fn(), checkout: vi.fn(), portal: vi.fn() }))
vi.mock('../api', () => ({ billingApi: a }))

import { useBillingStore } from './billing'

describe('billing store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads the summary; invoices failing never hides the plan; 403 is forbidden', async () => {
    const store = useBillingStore()
    a.summary.mockResolvedValueOnce({ enabled: true, usage: [] })
    a.invoices.mockRejectedValueOnce(new Error('stripe down'))
    await store.load()
    expect(store.state).toBe('ready')
    expect(store.invoices).toEqual([])
    a.summary.mockRejectedValueOnce({ response: { status: 403 } })
    await store.load()
    expect(store.state).toBe('forbidden')
  })

  it('returns the Stripe URL, or the server message', async () => {
    const store = useBillingStore()
    a.checkout.mockResolvedValueOnce({ url: 'https://checkout.stripe.com/c/1' })
    expect(await store.checkout('starter')).toBe('https://checkout.stripe.com/c/1')
    expect(a.checkout).toHaveBeenCalledWith('starter')
    a.portal.mockRejectedValueOnce({ response: { status: 400, data: { message: 'choose a plan first' } } })
    expect(await store.portal()).toBeNull()
    expect(store.actionError).toBe('choose a plan first')
  })
})
