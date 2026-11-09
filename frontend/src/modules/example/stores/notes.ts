import { defineStore } from 'pinia'
import { ref } from 'vue'
import { notesApi, type Note } from '../api'

/** What the notes page should show. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

function messageOf(err: unknown): string | undefined {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message
}

export const useExampleNotesStore = defineStore('wl-example-notes', () => {
  const notes = ref<Note[]>([])
  const state = ref<LoadState>('idle')
  const saving = ref(false)
  /** Message from the last failed save, if any (validation or permission). */
  const saveError = ref<string | null>(null)
  /** True when the last save was refused for lack of permission. */
  const saveForbidden = ref(false)

  async function fetchNotes() {
    state.value = 'loading'
    try {
      notes.value = await notesApi.list()
      state.value = 'ready'
    } catch (err) {
      notes.value = []
      state.value = statusOf(err) === 403 ? 'forbidden' : 'error'
    }
  }

  /** Adds a note; returns true on success. */
  async function addNote(text: string): Promise<boolean> {
    saving.value = true
    saveError.value = null
    saveForbidden.value = false
    try {
      const note = await notesApi.create(text)
      notes.value = [note, ...notes.value]
      return true
    } catch (err) {
      saveForbidden.value = statusOf(err) === 403
      saveError.value = messageOf(err) ?? null
      return false
    } finally {
      saving.value = false
    }
  }

  return { notes, state, saving, saveError, saveForbidden, fetchNotes, addNote }
})
