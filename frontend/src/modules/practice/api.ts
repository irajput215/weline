// Calls to /api/x/practice through Weline's shared client (cookies, CSRF,
// organization switching and token refresh come with it). Spec:
// wl-docs/modules/practice/api.md
import { api } from '@/services/api'

export interface Profile {
  id: string
  name: string
  time_zone: string
  booking_slug: string
  slot_minutes: number
  abn: string
  phone: string
  email: string
  website: string
}

export type ProfileInput = Omit<Profile, 'id'>

export interface ProfileResponse {
  profile: Profile | null
  setup: { missing: string[] }
  time_zones: string[]
  slot_lengths: number[]
}

export interface Location {
  id: string
  name: string
  address: string
  phone: string
  is_active: boolean
}

export interface Practitioner {
  id: string
  display_name: string
  role: string
  ahpra_number: string
  user_id: string | null
  user_name?: string
  location_ids: string[]
  is_active: boolean
}

export interface Member {
  id: string
  full_name: string
  email: string
}

export interface Service {
  id: string
  name: string
  duration_minutes: number
  price_cents: number | null
  allowed_roles: string[]
  is_active: boolean
}

export interface Hours {
  weekday: number // 1 = Monday … 7 = Sunday
  opens_at: string // "HH:MM"
  closes_at: string
}

export interface Closure {
  id: string
  location_id: string | null
  starts_on: string
  ends_on: string
  reason: string
}

const base = '/x/practice'
const data = <T>(p: Promise<{ data: { data: T } }>) => p.then(r => r.data.data)
const inactive = (all: boolean) => (all ? { params: { include_inactive: 'true' } } : undefined)

export const practiceApi = {
  getProfile: () => data<ProfileResponse>(api.get(`${base}/profile`)),
  saveProfile: (body: ProfileInput) => data<{ profile: Profile }>(api.put(`${base}/profile`, body)),

  listLocations: (all = true) => data<{ locations: Location[] }>(api.get(`${base}/locations`, inactive(all))),
  createLocation: (body: Partial<Location>) => data<{ location: Location }>(api.post(`${base}/locations`, body)),
  updateLocation: (id: string, body: Partial<Location>) => data<{ location: Location }>(api.put(`${base}/locations/${id}`, body)),

  listPractitioners: (all = true) => data<{ practitioners: Practitioner[] }>(api.get(`${base}/practitioners`, inactive(all))),
  createPractitioner: (body: Partial<Practitioner>) => data<{ practitioner: Practitioner }>(api.post(`${base}/practitioners`, body)),
  updatePractitioner: (id: string, body: Partial<Practitioner>) =>
    data<{ practitioner: Practitioner }>(api.put(`${base}/practitioners/${id}`, body)),
  listMembers: () => data<{ members: Member[] }>(api.get(`${base}/members`)),

  listServices: (all = true) => data<{ services: Service[] }>(api.get(`${base}/services`, inactive(all))),
  createService: (body: Partial<Service>) => data<{ service: Service; warnings: string[] }>(api.post(`${base}/services`, body)),
  updateService: (id: string, body: Partial<Service>) =>
    data<{ service: Service; warnings: string[] }>(api.put(`${base}/services/${id}`, body)),

  getHours: (locationId: string) => data<{ hours: Hours[] }>(api.get(`${base}/locations/${locationId}/hours`)),
  setHours: (locationId: string, hours: Hours[]) => data<{ hours: Hours[] }>(api.put(`${base}/locations/${locationId}/hours`, { hours })),
  listClosures: (from?: string) => data<{ closures: Closure[] }>(api.get(`${base}/closures`, from ? { params: { from } } : undefined)),
  createClosure: (body: Omit<Closure, 'id'>) => data<{ closure: Closure }>(api.post(`${base}/closures`, body)),
  deleteClosure: (id: string) => data<{ deleted: boolean }>(api.delete(`${base}/closures/${id}`))
}
