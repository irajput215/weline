import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ settings: vi.fn(), saveSettings: vi.fn(), knowledge: vi.fn(), addKnowledge: vi.fn(), deleteKnowledge: vi.fn(), runs: vi.fn() }))
vi.mock('../api', () => ({ aiApi: a }))

import { useAiStore } from './ai'

describe('ai store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads each part with its own state; 403 is forbidden', async () => {
    const store = useAiStore()
    a.settings.mockResolvedValue({ settings: { enabled: false }, provider: { key_set: false } })
    await store.loadSettings()
    expect(store.state.settings).toBe('ready')
    expect(store.provider?.key_set).toBe(false)
    a.knowledge.mockRejectedValueOnce({ response: { status: 403 } })
    await store.loadKnowledge()
    expect(store.state.knowledge).toBe('forbidden')
    a.runs.mockResolvedValue({ runs: null, next_before: null })
    await store.loadRuns()
    expect(store.runs).toEqual([])
  })

  it('adds and removes knowledge, keeping the server message on failure', async () => {
    const store = useAiStore()
    a.addKnowledge.mockResolvedValueOnce({ item: { source_id: 's1', title: 'Parking?' } })
    expect(await store.addFaq('Parking?', 'Bay Lane')).toBe(true)
    expect(a.addKnowledge).toHaveBeenCalledWith({ kind: 'faq', question: 'Parking?', answer: 'Bay Lane' })
    expect(store.items.map(i => i.source_id)).toEqual(['s1'])
    a.addKnowledge.mockRejectedValueOnce({ response: { status: 400, data: { message: 'text: at most 200 KB' } } })
    expect(await store.addDocument('Big', 'x')).toBe(false)
    expect(store.saveError).toBe('text: at most 200 KB')
    a.deleteKnowledge.mockResolvedValueOnce({ deleted: true })
    await store.remove('s1')
    expect(store.items).toEqual([])
  })
})
