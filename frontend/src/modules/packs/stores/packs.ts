import { defineStore } from 'pinia'
import { ref } from 'vue'
import { packsApi, type InstallResult, type Pack, type PackSummary } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status
const messageOf = (err: unknown) => (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''

/** Industry packs: list, preview, install. */
export const usePacksStore = defineStore('wl-packs', () => {
  const state = ref<LoadState>('idle')
  const packs = ref<PackSummary[]>([])
  const preview = ref<Pack | null>(null)
  const result = ref<InstallResult | null>(null)
  const installing = ref(false)
  const installError = ref<string | null>(null)

  async function load() {
    state.value = 'loading'
    try {
      packs.value = (await packsApi.list()).packs ?? []
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function open(id: string) {
    result.value = null
    installError.value = null
    try {
      preview.value = (await packsApi.get(id)).pack
    } catch {
      preview.value = null
      installError.value = 'error'
    }
  }

  async function install(id: string) {
    installing.value = true
    installError.value = null
    try {
      result.value = (await packsApi.install(id)).result
      const p = packs.value.find(x => x.id === id)
      if (p) p.installed = result.value
      return true
    } catch (err) {
      installError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return false
    } finally {
      installing.value = false
    }
  }

  return { state, packs, preview, result, installing, installError, load, open, install }
})
