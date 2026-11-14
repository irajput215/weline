import { defineStore } from 'pinia'
import { ref } from 'vue'
import { timelineApi, type ContactRef, type Timeline } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden' | 'notfound'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status

/** Contact timeline: search a contact, then page through their history. */
export const useTimelineStore = defineStore('wl-timeline', () => {
  const results = ref<ContactRef[]>([])
  const searchState = ref<LoadState>('idle')
  const state = ref<LoadState>('idle')
  const timeline = ref<Timeline | null>(null)
  const loadingMore = ref(false)

  async function search(q: string) {
    if (q.trim().length < 2) {
      results.value = []
      searchState.value = 'idle'
      return
    }
    searchState.value = 'loading'
    try {
      results.value = (await timelineApi.search(q)).contacts ?? []
      searchState.value = 'ready'
    } catch (err) {
      searchState.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function open(id: string) {
    state.value = 'loading'
    try {
      timeline.value = await timelineApi.get(id)
      state.value = 'ready'
    } catch (err) {
      const s = statusOf(err)
      state.value = s === 403 ? 'forbidden' : s === 404 ? 'notfound' : 'error'
    }
  }

  async function more() {
    const t = timeline.value
    if (!t?.next_before) return
    loadingMore.value = true
    try {
      const next = await timelineApi.get(t.contact.id, t.next_before)
      t.items = [...t.items, ...next.items]
      t.next_before = next.next_before
    } finally {
      loadingMore.value = false
    }
  }

  return { results, searchState, state, timeline, loadingMore, search, open, more }
})
