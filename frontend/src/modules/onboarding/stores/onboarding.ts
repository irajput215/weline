import { defineStore } from 'pinia'
import { ref } from 'vue'
import { onboardingApi, type Checklist, type Vertical } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status
const messageOf = (err: unknown) => (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''

/** Onboarding module: the setup checklist. */
export const useOnboardingStore = defineStore('wl-onboarding', () => {
  const state = ref<LoadState>('idle')
  const checklist = ref<Checklist | null>(null)
  const saving = ref(false)
  const saveError = ref<string | null>(null)

  async function load() {
    state.value = 'loading'
    try {
      checklist.value = await onboardingApi.checklist()
      state.value = 'ready'
    } catch (err) {
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function save(fn: () => Promise<Checklist>) {
    saving.value = true
    saveError.value = null
    try {
      checklist.value = await fn()
      return true
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return false
    } finally {
      saving.value = false
    }
  }

  const setVertical = (v: Vertical) => save(() => onboardingApi.setVertical(v))
  const setDismissed = (d: boolean) => save(() => onboardingApi.setDismissed(d))

  return { state, checklist, saving, saveError, load, setVertical, setDismissed }
})
