// Weline-only API client for the booking feature (backend: internal/booking).
import { api } from '@/services/api'

export interface Appointment {
  id: string
  organization_id: string
  contact_id?: string
  starts_at: string
  ends_at: string
  status: string
  notes: string
  created_at: string
  updated_at: string
}

export const bookingService = {
  listAppointments: () =>
    api.get<{ data: { appointments: Appointment[] } }>('/booking/appointments')
}
