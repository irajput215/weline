import { defineStore } from 'pinia'
import { ref } from 'vue'
import { billingApi, type Invoice, type Summary } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status
const messageOf = (err: unknown) => (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''

/** Billing module: plan, usage, invoices; checkout and portal leave for Stripe. */
export const useBillingStore = defineStore('wl-billing', () => {
  const state = ref<LoadState>('idle')
  const summary = ref<Summary | null>(null)
  const invoices = ref<Invoice[]>([])
  const busy = ref(false)
  const actionError = ref<string | null>(null)

  async function load() {
    state.value = 'loading'
    try {
      summary.value = await billingApi.summary()
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
      return
    }
    try {
      invoices.value = (await billingApi.invoices()).invoices ?? []
    } catch {
      invoices.value = [] // the plan still shows if Stripe is unreachable
    }
  }

  /** Gets a Stripe URL to go to, or null with actionError set. */
  async function go(fn: () => Promise<{ url: string }>): Promise<string | null> {
    busy.value = true
    actionError.value = null
    try {
      return (await fn()).url
    } catch (err) {
      actionError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return null
    } finally {
      busy.value = false
    }
  }

  const checkout = (plan: string) => go(() => billingApi.checkout(plan))
  const portal = () => go(() => billingApi.portal())

  return { state, summary, invoices, busy, actionError, load, checkout, portal }
})
