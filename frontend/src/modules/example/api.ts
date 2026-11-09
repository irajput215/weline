// Calls to /api/x/example, through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it).
import { api } from '@/services/api'

export interface Note {
  id: string
  organization_id: string
  text: string
  created_by_id?: string
  created_at: string
}

export const notesApi = {
  async list(): Promise<Note[]> {
    const res = await api.get('/x/example/notes')
    return res.data.data.notes
  },
  async create(text: string): Promise<Note> {
    const res = await api.post('/x/example/notes', { text })
    return res.data.data.note
  }
}
