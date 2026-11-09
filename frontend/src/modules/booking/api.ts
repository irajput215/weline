// Calls to /api/x/booking through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it). Spec:
// wl-docs/modules/booking/api.md
import { api } from '@/services/api'

export type Status = 'BOOKED' | 'CONFIRMED' | 'ARRIVED' | 'COMPLETED' | 'CANCELLED' | 'NO_SHOW'

export interface Ref {
  id: string
  name: string
}

export interface Patient {
  id: string
  name: string
  phone: string
}

export interface Appointment {
  id: string
  reference: string
  status: Status
  source: 'staff' | 'web' | 'whatsapp' | 'cliniko'
  starts_at: string
  ends_at: string
  local: string // "2026-11-11T10:15", clinic time
  label: string
  contact: Patient
  practitioner: Ref
  service: Ref
  duration_minutes: number
  location: Ref
  note: string
  outside_hours: boolean
  created_at: string
  allowed_next: Status[]
}

export interface HistoryEntry {
  from_status: Status | null
  to_status: Status
  changes: Record<string, unknown>
  changed_by: Ref | null
  source: string
  reason: string
  created_at: string
}

export interface Slot {
  starts_at: string
  local: string
  label: string
  practitioner_ids: string[]
}

export interface Availability {
  location_id: string
  weekday: number // 1 = Monday … 7 = Sunday
  starts_at: string // "HH:MM"
  ends_at: string
}

export interface Block {
  id: string
  practitioner_id: string
  starts_at: string
  ends_at: string
  reason: string
  created_at: string
}

export interface Settings {
  min_notice_minutes: number
  window_days: number
}

export interface Readiness {
  ok: boolean
  missing: string[] // profile, services, availability, captcha
  slug: string
  path: string
}

export interface NewAppointment {
  contact_id: string
  service_id: string
  location_id: string
  practitioner_id: string
  starts_at: string
  note?: string
  outside_hours?: boolean
  allow_patient_overlap?: boolean
}

export interface SlotQuery {
  service_id: string
  location_id: string
  practitioner_id?: string
  from: string
  to?: string
}

export interface ListQuery {
  from: string
  to?: string
  practitioner_id?: string
  location_id?: string
  status?: Status
}

const base = '/x/booking'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)
const params = (q: object) => ({ params: Object.fromEntries(Object.entries(q).filter(([, v]) => v !== undefined && v !== '')) })

export const bookingApi = {
  slots: (q: SlotQuery) => data<{ slots: Slot[] }>(api.get(`${base}/slots`, params(q))),

  listAppointments: (q: ListQuery) => data<{ appointments: Appointment[] }>(api.get(`${base}/appointments`, params(q))),
  getAppointment: (id: string) => data<{ appointment: Appointment; history: HistoryEntry[] }>(api.get(`${base}/appointments/${id}`)),
  createAppointment: (body: NewAppointment) =>
    data<{ appointment: Appointment; confirmation: string }>(api.post(`${base}/appointments`, body)),
  moveAppointment: (id: string, body: { starts_at?: string; practitioner_id?: string; location_id?: string; note?: string; outside_hours?: boolean }) =>
    data<{ appointment: Appointment }>(api.put(`${base}/appointments/${id}`, body)),
  setStatus: (id: string, status: Status, reason = '') =>
    data<{ appointment: Appointment }>(api.post(`${base}/appointments/${id}/status`, { status, reason })),

  searchPatients: (q: string) => data<{ patients: Patient[] }>(api.get(`${base}/patients`, { params: { q } })),
  addPatient: (name: string, mobile: string) => data<{ patient: Patient; existing: boolean }>(api.post(`${base}/patients`, { name, mobile })),

  getAvailability: (practitionerId: string) => data<{ blocks: Availability[] }>(api.get(`${base}/practitioners/${practitionerId}/availability`)),
  setAvailability: (practitionerId: string, blocks: Availability[]) =>
    data<{ blocks: Availability[]; warnings: string[] }>(api.put(`${base}/practitioners/${practitionerId}/availability`, { blocks })),
  listBlocks: (practitionerId?: string) => data<{ blocks: Block[] }>(api.get(`${base}/blocks`, params({ practitioner_id: practitionerId }))),
  createBlock: (body: { practitioner_id: string; starts_at: string; ends_at: string; reason: string }) =>
    data<{ block: Block; clashes: Appointment[] }>(api.post(`${base}/blocks`, body)),
  deleteBlock: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/blocks/${id}`)),

  getSettings: () => data<{ settings: Settings; ready: Readiness }>(api.get(`${base}/settings`)),
  saveSettings: (body: Settings) => data<{ settings: Settings; ready: Readiness }>(api.put(`${base}/settings`, body))
}
