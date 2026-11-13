import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { remindersApi, type Due, type Rule, type Send, type SendCounts, type TemplateOption } from '../api'

export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Part = 'rules' | 'sends' | 'due'

const statusOf = (err: unknown) => (err as { response?: { status?: number } })?.response?.status
const messageOf = (err: unknown) => (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''

/** Reminders module: rules, what was sent, and recalls due. */
export const useRemindersStore = defineStore('wl-reminders', () => {
  const state = reactive<Record<Part, LoadState>>({ rules: 'idle', sends: 'idle', due: 'idle' })
  const rules = ref<Rule[]>([])
  const templates = ref<TemplateOption[]>([])
  const sends = ref<Send[]>([])
  const nextBefore = ref<string | null>(null)
  const due = ref<Due[]>([])
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

  const loadRules = () => load('rules', async () => {
    rules.value = (await remindersApi.rules()).rules ?? []
  })
  async function loadTemplates() {
    try {
      templates.value = await remindersApi.templates()
    } catch {
      templates.value = [] // no templates permission: the name can still be typed
    }
  }
  const loadSends = () => load('sends', async () => {
    const r = await remindersApi.sends()
    sends.value = r.sends ?? []
    nextBefore.value = r.next_before
  })
  async function loadMore() {
    if (!nextBefore.value) return
    const r = await remindersApi.sends(nextBefore.value)
    sends.value = [...sends.value, ...(r.sends ?? [])]
    nextBefore.value = r.next_before
  }
  const loadDue = (ruleId: string) => load('due', async () => {
    due.value = (await remindersApi.due(ruleId)).due ?? []
  })

  async function saveRule(body: Rule) {
    const r = await save(() => (body.id ? remindersApi.updateRule(body.id, body) : remindersApi.createRule(body)))
    if (!r) return false
    const i = rules.value.findIndex(x => x.id === r.rule.id)
    if (i >= 0) rules.value.splice(i, 1, r.rule)
    else rules.value = [...rules.value, r.rule]
    return true
  }
  async function removeRule(id: string) {
    const r = await save(() => remindersApi.deleteRule(id))
    if (r) rules.value = rules.value.filter(x => x.id !== id)
    return !!r
  }
  async function sendRecalls(ruleId: string, ids: string[]): Promise<SendCounts | null> {
    const r = await save(() => remindersApi.sendRecalls(ruleId, ids))
    if (!r) return null
    due.value = due.value.filter(d => !ids.includes(d.customer_id))
    return r.counts
  }

  return { state, rules, templates, sends, nextBefore, due, saving, saveError, loadRules, loadTemplates, loadSends, loadMore, loadDue, saveRule, removeRule, sendRecalls }
})
