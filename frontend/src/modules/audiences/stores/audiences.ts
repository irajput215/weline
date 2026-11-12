import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import {
  audiencesApi,
  type Condition,
  type Field,
  type Fields,
  type Preview,
  type PreviewInput,
  type Rules,
  type Segment,
  type Send,
  type SendInput,
  type SendResult
} from '../api'

/** What an audiences page should show for one resource. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Resource = 'segments' | 'segment' | 'fields' | 'sends'

/** The operators each field allows (design.md → Rules). */
export const OPS: Record<Field, string[]> = {
  tag: ['has', 'has_not'],
  metadata: ['eq', 'neq', 'contains', 'exists', 'not_exists'],
  whatsapp_account: ['eq'],
  last_inbound: ['within_days', 'not_within_days'],
  created: ['within_days', 'not_within_days'],
  campaign: ['received', 'not_received', 'read', 'not_read', 'read_no_reply', 'replied'],
  consent: ['opted_in', 'opted_out', 'not_recorded']
}
export const FIELDS = Object.keys(OPS) as Field[]
export const RETARGET_FILTERS = ['not_received', 'not_read', 'read_no_reply', 'replied'] as const
export const MAX_CONDITIONS = 20

/** A fresh condition row for a field, with sensible defaults. */
export function newCondition(field: Field = 'tag'): Condition {
  const c: Condition = { field, op: OPS[field][0] }
  if (field === 'last_inbound' || field === 'created') c.value = 90
  else if (field === 'metadata') c.key = ''
  if (field !== 'last_inbound' && field !== 'created' && field !== 'consent' && !(field === 'metadata' && (c.op === 'exists' || c.op === 'not_exists'))) {
    c.value = c.value ?? ''
  }
  return c
}

/** Whether a row has what the server needs (no request for half-filled rows). */
export function isComplete(c: Condition): boolean {
  if (!OPS[c.field]?.includes(c.op)) return false
  if (c.field === 'consent') return true
  if (c.field === 'metadata') {
    if (!/^[a-zA-Z0-9_]{1,50}$/.test(c.key ?? '')) return false
    if (c.op === 'exists' || c.op === 'not_exists') return true
    return typeof c.value === 'string'
  }
  if (c.field === 'last_inbound' || c.field === 'created') {
    const n = Number(c.value)
    return Number.isInteger(n) && n >= 1 && n <= 365
  }
  return typeof c.value === 'string' && c.value.trim() !== ''
}

/** Rules as the server takes them: numbers as numbers, unused parts dropped. */
export function cleanRules(r: Rules): Rules {
  return {
    match: r.match,
    conditions: r.conditions.map(c => {
      const out: Condition = { field: c.field, op: c.op }
      if (c.field === 'metadata') out.key = c.key
      if (c.field === 'last_inbound' || c.field === 'created') out.value = Number(c.value)
      else if (c.field === 'consent') return out
      else if (!(c.field === 'metadata' && (c.op === 'exists' || c.op === 'not_exists'))) out.value = String(c.value ?? '').trim()
      if (c.field === 'campaign' && c.days) out.days = Number(c.days)
      return out
    })
  }
}

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** Audiences module: one store for its pages. */
export const useAudiencesStore = defineStore('wl-audiences', () => {
  const state = reactive<Record<Resource, LoadState>>({ segments: 'idle', segment: 'idle', fields: 'idle', sends: 'idle' })

  const segments = ref<Segment[]>([])
  const segment = ref<Segment | null>(null)
  const fields = ref<Fields | null>(null)
  const sends = ref<Send[]>([])

  const preview = ref<Preview | null>(null)
  const previewLoading = ref(false)
  const previewError = ref<string | null>(null)
  let previewSeq = 0

  const saving = ref(false)
  /** Message from the last failed save (validation, conflict or permission). */
  const saveError = ref<string | null>(null)

  async function load(resource: Resource, fn: () => Promise<void>) {
    state[resource] = 'loading'
    try {
      await fn()
      state[resource] = 'ready'
    } catch (err) {
      state[resource] = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  /** Runs a save; returns its result, or null after recording the error. */
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

  const loadSegments = () => load('segments', async () => {
    segments.value = (await audiencesApi.listSegments()).segments ?? []
  })

  const loadSegment = (id: string) => load('segment', async () => {
    segment.value = (await audiencesApi.getSegment(id)).segment
  })

  const loadFields = () => load('fields', async () => {
    fields.value = await audiencesApi.fields()
  })

  const loadSends = (q: { segment_id?: string; campaign_id?: string } = {}) => load('sends', async () => {
    sends.value = (await audiencesApi.listSends(q)).sends ?? []
  })

  /** Counts what rules or a retarget reach. Only the latest answer is kept. */
  async function runPreview(input: PreviewInput) {
    const seq = ++previewSeq
    if (input.rules && (input.rules.conditions.length === 0 || !input.rules.conditions.every(isComplete))) {
      preview.value = null
      previewError.value = null
      previewLoading.value = false
      return
    }
    previewLoading.value = true
    previewError.value = null
    try {
      const body = input.rules ? { ...input, rules: cleanRules(input.rules) } : input
      const p = await audiencesApi.preview(body)
      if (seq === previewSeq) preview.value = p
    } catch (err) {
      if (seq === previewSeq) {
        preview.value = null
        previewError.value = messageOf(err) || 'error'
      }
    } finally {
      if (seq === previewSeq) previewLoading.value = false
    }
  }

  function resetPreview() {
    previewSeq++
    preview.value = null
    previewError.value = null
    previewLoading.value = false
  }

  /** Creates or updates a segment; returns it, or null on failure. */
  async function saveSegment(id: string | null, body: { name: string; description: string; rules: Rules }) {
    const res = await save(() =>
      id ? audiencesApi.updateSegment(id, { ...body, rules: cleanRules(body.rules) })
        : audiencesApi.createSegment({ ...body, rules: cleanRules(body.rules) })
    )
    if (res) segment.value = res.segment
    return res?.segment ?? null
  }

  async function deleteSegment(id: string) {
    const res = await save(() => audiencesApi.deleteSegment(id))
    if (res) segments.value = segments.value.filter(s => s.id !== id)
    return !!res
  }

  /**
   * Copies a segment under the first free name from names (R3). Returns the copy,
   * or null after recording the error.
   */
  async function duplicateSegment(source: Segment, names: string[]): Promise<Segment | null> {
    saving.value = true
    saveError.value = null
    try {
      for (const name of names) {
        try {
          const res = await audiencesApi.createSegment({ name, description: source.description, rules: source.rules })
          segments.value = [...segments.value, { ...res.segment, count: source.count }].sort((a, b) => a.name.localeCompare(b.name))
          return res.segment
        } catch (err) {
          if (statusOf(err) !== 409) throw err
        }
      }
      saveError.value = 'error'
      return null
    } catch (err) {
      saveError.value = messageOf(err) || (statusOf(err) === 403 ? 'forbidden' : 'error')
      return null
    } finally {
      saving.value = false
    }
  }

  async function createSend(body: SendInput): Promise<SendResult | null> {
    return save(() => audiencesApi.createSend(body))
  }

  return {
    state, segments, segment, fields, sends, preview, previewLoading, previewError, saving, saveError,
    loadSegments, loadSegment, loadFields, loadSends, runPreview, resetPreview, saveSegment, deleteSegment, duplicateSegment, createSend
  }
})

/** A date for lists, in the browser's locale. */
export function formatDate(iso: string | null | undefined, withTime = false): string {
  if (!iso) return ''
  const opts: Intl.DateTimeFormatOptions = withTime
    ? { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' }
    : { day: 'numeric', month: 'short', year: 'numeric' }
  return new Intl.DateTimeFormat(undefined, opts).format(new Date(iso))
}
