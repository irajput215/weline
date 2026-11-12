import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({
  listSegments: vi.fn(),
  getSegment: vi.fn(),
  createSegment: vi.fn(),
  updateSegment: vi.fn(),
  deleteSegment: vi.fn(),
  preview: vi.fn(),
  fields: vi.fn(),
  createSend: vi.fn(),
  listSends: vi.fn()
}))
vi.mock('../api', () => ({ audiencesApi: a }))

import { cleanRules, isComplete, newCondition, useAudiencesStore } from './audiences'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })
const preview = (recipients: number) => ({ matched: recipients, recipients, excluded: {}, sample: [] })

describe('audiences store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads segments and treats null as empty', async () => {
    a.listSegments.mockResolvedValue({ segments: null })
    const store = useAudiencesStore()
    await store.loadSegments()
    expect(store.state.segments).toBe('ready')
    expect(store.segments).toEqual([])
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useAudiencesStore()
    a.listSegments.mockRejectedValueOnce(httpError(403))
    await store.loadSegments()
    expect(store.state.segments).toBe('forbidden')
    a.fields.mockRejectedValueOnce(httpError(500))
    await store.loadFields()
    expect(store.state.fields).toBe('error')
  })

  it('does not preview half-filled rules', async () => {
    const store = useAudiencesStore()
    await store.runPreview({ rules: { match: 'all', conditions: [newCondition('tag')] } })
    expect(a.preview).not.toHaveBeenCalled()
    expect(store.preview).toBeNull()
  })

  it('keeps only the latest preview', async () => {
    let first: (v: unknown) => void = () => {}
    a.preview.mockReturnValueOnce(new Promise(r => (first = r))).mockResolvedValueOnce(preview(2))
    const store = useAudiencesStore()
    const rules = { match: 'all' as const, conditions: [{ field: 'tag' as const, op: 'has', value: 'vip' }] }
    const p1 = store.runPreview({ rules })
    await store.runPreview({ rules })
    first(preview(9))
    await p1
    expect(store.preview?.recipients).toBe(2)
    expect(store.previewLoading).toBe(false)
  })

  it('records the server message when a save fails', async () => {
    a.createSegment.mockRejectedValueOnce(httpError(409, 'a segment named "VIP" already exists'))
    const store = useAudiencesStore()
    const seg = await store.saveSegment(null, { name: 'VIP', description: '', rules: { match: 'all', conditions: [] } })
    expect(seg).toBeNull()
    expect(store.saveError).toContain('already exists')
    a.createSend.mockRejectedValueOnce(httpError(403))
    expect(await store.createSend({} as never)).toBeNull()
    expect(store.saveError).toBe('forbidden')
  })

  it('removes a deleted segment from the list', async () => {
    a.listSegments.mockResolvedValue({ segments: [{ id: 's1' }, { id: 's2' }] })
    a.deleteSegment.mockResolvedValue({ deleted: true })
    const store = useAudiencesStore()
    await store.loadSegments()
    expect(await store.deleteSegment('s1')).toBe(true)
    expect(store.segments.map(s => s.id)).toEqual(['s2'])
  })
})

describe('rules helpers', () => {
  it('checks rows are complete', () => {
    expect(isComplete({ field: 'tag', op: 'has', value: 'vip' })).toBe(true)
    expect(isComplete({ field: 'tag', op: 'eq', value: 'vip' })).toBe(false)
    expect(isComplete({ field: 'metadata', op: 'exists', key: 'suburb' })).toBe(true)
    expect(isComplete({ field: 'metadata', op: 'eq', key: 'bad key', value: 'x' })).toBe(false)
    expect(isComplete({ field: 'last_inbound', op: 'within_days', value: 0 })).toBe(false)
    expect(isComplete({ field: 'created', op: 'within_days', value: 30 })).toBe(true)
  })

  it('cleans rules for the server', () => {
    const r = cleanRules({
      match: 'any',
      conditions: [
        { field: 'last_inbound', op: 'within_days', value: '30' as unknown as number, key: 'x' },
        { field: 'metadata', op: 'exists', key: 'plan', value: 'ignored' },
        { field: 'tag', op: 'has', value: ' vip ' }
      ]
    })
    expect(r.conditions).toEqual([
      { field: 'last_inbound', op: 'within_days', value: 30 },
      { field: 'metadata', op: 'exists', key: 'plan' },
      { field: 'tag', op: 'has', value: 'vip' }
    ])
  })
})
