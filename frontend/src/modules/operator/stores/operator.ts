import { defineStore } from 'pinia'
import { ref } from 'vue'
import { operatorApi, type Overview, type Tenant, type TenantDetail } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status

/** Operator console: overview, organizations and one organization's detail. */
export const useOperatorStore = defineStore('wl-operator', () => {
  const state = ref<LoadState>('idle')
  const overview = ref<Overview | null>(null)
  const tenants = ref<Tenant[]>([])
  const nextOffset = ref<number | null>(null)
  const search = ref('')
  const detail = ref<TenantDetail | null>(null)
  const detailState = ref<LoadState>('idle')

  async function load() {
    state.value = 'loading'
    try {
      const [o, t] = await Promise.all([operatorApi.overview(), operatorApi.tenants(search.value)])
      overview.value = o
      tenants.value = t.tenants ?? []
      nextOffset.value = t.next_offset
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function find(q: string) {
    search.value = q
    try {
      const t = await operatorApi.tenants(q)
      tenants.value = t.tenants ?? []
      nextOffset.value = t.next_offset
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function loadMore() {
    if (nextOffset.value === null) return
    const t = await operatorApi.tenants(search.value, nextOffset.value)
    tenants.value = [...tenants.value, ...(t.tenants ?? [])]
    nextOffset.value = t.next_offset
  }

  async function open(id: string) {
    detailState.value = 'loading'
    detail.value = null
    try {
      detail.value = await operatorApi.tenant(id)
      detailState.value = 'ready'
    } catch (err) {
      detailState.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  return { state, overview, tenants, nextOffset, search, detail, detailState, load, find, loadMore, open }
})
