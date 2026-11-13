import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { aiApi, type AiSettings, type KnowledgeItem, type ProviderInfo, type Run } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Part = 'settings' | 'knowledge' | 'runs'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status
export const messageOf = (err: unknown) => (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''

/** AI agent module: settings, knowledge and activity. */
export const useAiStore = defineStore('wl-ai', () => {
  const state = reactive<Record<Part, LoadState>>({ settings: 'idle', knowledge: 'idle', runs: 'idle' })
  const settings = ref<AiSettings | null>(null)
  const provider = ref<ProviderInfo | null>(null)
  const items = ref<KnowledgeItem[]>([])
  const runs = ref<Run[]>([])
  const nextBefore = ref<string | null>(null)
  const saving = ref(false)
  const saveError = ref<string | null>(null)

  async function load(part: Part, fn: () => Promise<void>) {
    state[part] = 'loading'
    try {
      await fn()
      state[part] = 'ready'
    } catch (err) {
      state[part] = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  async function save<T>(fn: () => Promise<T>): Promise<T | null> {
    saving.value = true
    saveError.value = null
    try {
      return await fn()
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return null
    } finally {
      saving.value = false
    }
  }

  const loadSettings = () => load('settings', async () => {
    const r = await aiApi.settings()
    settings.value = r.settings
    provider.value = r.provider
  })
  const loadKnowledge = () => load('knowledge', async () => {
    items.value = (await aiApi.knowledge()).items ?? []
  })
  const loadRuns = () => load('runs', async () => {
    const r = await aiApi.runs()
    runs.value = r.runs ?? []
    nextBefore.value = r.next_before
  })

  async function saveSettings(body: AiSettings) {
    const r = await save(() => aiApi.saveSettings(body))
    if (r) settings.value = r.settings
    return !!r
  }
  async function addFaq(question: string, answer: string) {
    const r = await save(() => aiApi.addKnowledge({ kind: 'faq', question, answer }))
    if (r) items.value = [r.item, ...items.value]
    return !!r
  }
  async function addDocument(title: string, text: string) {
    const r = await save(() => aiApi.addKnowledge({ kind: 'document', title, text }))
    if (r) items.value = [r.item, ...items.value]
    return !!r
  }
  async function remove(id: string) {
    const r = await save(() => aiApi.deleteKnowledge(id))
    if (r) items.value = items.value.filter(i => i.source_id !== id)
    return !!r
  }
  async function loadMore() {
    if (!nextBefore.value) return
    const r = await aiApi.runs(nextBefore.value)
    runs.value = [...runs.value, ...(r.runs ?? [])]
    nextBefore.value = r.next_before
  }

  return { state, settings, provider, items, runs, nextBefore, saving, saveError, loadSettings, loadKnowledge, loadRuns, saveSettings, addFaq, addDocument, remove, loadMore }
})
