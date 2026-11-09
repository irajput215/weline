import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const list = vi.fn()
const create = vi.fn()
vi.mock('../api', () => ({ notesApi: { list: (...a: unknown[]) => list(...a), create: (...a: unknown[]) => create(...a) } }))

import { useExampleNotesStore } from './notes'

const note = (id: string, text: string) => ({ id, organization_id: 'org', text, created_at: '2026-11-09T00:00:00Z' })
const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })

describe('example notes store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    list.mockReset()
    create.mockReset()
  })

  it('loads notes', async () => {
    list.mockResolvedValue([note('1', 'Order gloves')])
    const store = useExampleNotesStore()

    const pending = store.fetchNotes()
    expect(store.state).toBe('loading')
    await pending

    expect(store.state).toBe('ready')
    expect(store.notes.map(n => n.text)).toEqual(['Order gloves'])
  })

  it('shows the forbidden state on 403 and the error state otherwise', async () => {
    const store = useExampleNotesStore()

    list.mockRejectedValueOnce(httpError(403))
    await store.fetchNotes()
    expect(store.state).toBe('forbidden')

    list.mockRejectedValueOnce(httpError(500))
    await store.fetchNotes()
    expect(store.state).toBe('error')
    expect(store.notes).toEqual([])
  })

  it('adds a note to the top of the list', async () => {
    list.mockResolvedValue([note('1', 'Older')])
    create.mockResolvedValue(note('2', 'Newer'))
    const store = useExampleNotesStore()
    await store.fetchNotes()

    expect(await store.addNote('Newer')).toBe(true)
    expect(create).toHaveBeenCalledWith('Newer')
    expect(store.notes.map(n => n.text)).toEqual(['Newer', 'Older'])
    expect(store.saving).toBe(false)
  })

  it('reports a refused save without changing the list', async () => {
    list.mockResolvedValue([])
    const store = useExampleNotesStore()
    await store.fetchNotes()

    create.mockRejectedValueOnce(httpError(403, 'Insufficient permissions'))
    expect(await store.addNote('nope')).toBe(false)
    expect(store.saveForbidden).toBe(true)
    expect(store.notes).toEqual([])

    create.mockRejectedValueOnce(httpError(400, 'note text is required'))
    expect(await store.addNote(' ')).toBe(false)
    expect(store.saveForbidden).toBe(false)
    expect(store.saveError).toBe('note text is required')
  })
})
