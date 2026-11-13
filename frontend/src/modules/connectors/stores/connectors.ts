import { defineStore } from 'pinia'
import { ref } from 'vue'
import { connectorsApi, type Account, type ProviderInfo, type Sync } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

export function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** Connectors module: the Connected system page. */
export const useConnectorsStore = defineStore('wl-connectors', () => {
  const state = ref<LoadState>('idle')
  const account = ref<Account | null>(null)
  const providers = ref<ProviderInfo[]>([])
  const syncs = ref<Sync[]>([])
  const saving = ref(false)
  const saveError = ref<string | null>(null)
  const testMessage = ref<{ ok: boolean; message: string } | null>(null)

  async function load() {
    state.value = 'loading'
    try {
      const [a, s] = await Promise.all([connectorsApi.account(), connectorsApi.syncs()])
      account.value = a.account
      providers.value = a.providers ?? []
      syncs.value = s.syncs ?? []
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function refreshSyncs() {
    try {
      syncs.value = (await connectorsApi.syncs()).syncs ?? []
    } catch {
      // keep what's shown
    }
  }

  async function save(provider: string, credentials?: Record<string, string>) {
    saving.value = true
    saveError.value = null
    testMessage.value = null
    try {
      const r = await connectorsApi.save({ provider, credentials })
      account.value = r.account
      return true
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return false
    } finally {
      saving.value = false
      void refreshSyncs()
    }
  }

  async function test() {
    saving.value = true
    saveError.value = null
    try {
      const r = await connectorsApi.test()
      account.value = r.account
      testMessage.value = { ok: r.ok, message: r.message }
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
    } finally {
      saving.value = false
      void refreshSyncs()
    }
  }

  return { state, account, providers, syncs, saving, saveError, testMessage, load, save, test }
})
