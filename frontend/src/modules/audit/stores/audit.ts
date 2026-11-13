import { defineStore } from 'pinia'
import { ref } from 'vue'
import { auditApi, type AuditEvent } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

/** Audit module: the trail of module changes. */
export const useAuditStore = defineStore('wl-audit', () => {
  const state = ref<LoadState>('idle')
  const events = ref<AuditEvent[]>([])
  const nextBefore = ref<string | null>(null)
  const module = ref('')
  const loadingMore = ref(false)

  async function load() {
    state.value = 'loading'
    try {
      const r = await auditApi.events({ module: module.value })
      events.value = r.events ?? []
      nextBefore.value = r.next_before
      state.value = 'ready'
    } catch (err) {
      state.value = (err as { response?: { status?: number } })?.response?.status === 403 ? 'forbidden' : 'error'
    }
  }

  async function loadMore() {
    if (!nextBefore.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const r = await auditApi.events({ module: module.value, before: nextBefore.value })
      events.value = [...events.value, ...(r.events ?? [])]
      nextBefore.value = r.next_before
    } finally {
      loadingMore.value = false
    }
  }

  return { state, events, nextBefore, module, loadingMore, load, loadMore }
})
