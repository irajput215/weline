import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import {
  practiceApi,
  type Closure,
  type Hours,
  type Location,
  type Member,
  type Practitioner,
  type Profile,
  type ProfileInput,
  type Service
} from '../api'

/** What a practice page should show for one resource. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Resource = 'profile' | 'locations' | 'practitioners' | 'services' | 'hours' | 'closures'

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** Clinic setup (practice module): one store for its five pages. */
export const usePracticeStore = defineStore('wl-practice', () => {
  const state = reactive<Record<Resource, LoadState>>({
    profile: 'idle',
    locations: 'idle',
    practitioners: 'idle',
    services: 'idle',
    hours: 'idle',
    closures: 'idle'
  })

  const profile = ref<Profile | null>(null)
  const setupMissing = ref<string[]>([])
  const timeZones = ref<string[]>([])
  const slotLengths = ref<number[]>([])
  const locations = ref<Location[]>([])
  const practitioners = ref<Practitioner[]>([])
  const members = ref<Member[]>([])
  const services = ref<Service[]>([])
  const hours = ref<Hours[]>([])
  const closures = ref<Closure[]>([])

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

  const replace = <T extends { id: string }>(list: T[], item: T) => {
    const i = list.findIndex(x => x.id === item.id)
    if (i >= 0) list.splice(i, 1, item)
    else list.push(item)
  }

  // profile
  const loadProfile = () =>
    load('profile', async () => {
      const r = await practiceApi.getProfile()
      profile.value = r.profile
      setupMissing.value = r.setup?.missing ?? []
      timeZones.value = r.time_zones
      slotLengths.value = r.slot_lengths
    })
  async function saveProfile(body: ProfileInput) {
    const r = await save(() => practiceApi.saveProfile(body))
    if (r) {
      profile.value = r.profile
      setupMissing.value = setupMissing.value.filter(s => s !== 'profile')
    }
    return r !== null
  }

  // locations
  const loadLocations = () =>
    load('locations', async () => {
      locations.value = (await practiceApi.listLocations()).locations
    })
  async function saveLocation(id: string | null, body: Partial<Location>) {
    const r = await save(() => (id ? practiceApi.updateLocation(id, body) : practiceApi.createLocation(body)))
    if (r) replace(locations.value, r.location)
    return r !== null
  }

  // practitioners
  const loadPractitioners = () =>
    load('practitioners', async () => {
      const [p, m] = await Promise.all([practiceApi.listPractitioners(), practiceApi.listMembers()])
      practitioners.value = p.practitioners
      members.value = m.members
    })
  async function savePractitioner(id: string | null, body: Partial<Practitioner>) {
    const r = await save(() => (id ? practiceApi.updatePractitioner(id, body) : practiceApi.createPractitioner(body)))
    if (r) replace(practitioners.value, r.practitioner)
    return r !== null
  }

  // services
  const loadServices = () =>
    load('services', async () => {
      services.value = (await practiceApi.listServices()).services
    })
  /** Saves a service; returns its warnings (e.g. roles nobody has), or null on failure. */
  async function saveService(id: string | null, body: Partial<Service>): Promise<string[] | null> {
    const r = await save(() => (id ? practiceApi.updateService(id, body) : practiceApi.createService(body)))
    if (!r) return null
    replace(services.value, r.service)
    return r.warnings ?? []
  }

  // hours and closures
  const loadHours = (locationId: string) =>
    load('hours', async () => {
      hours.value = (await practiceApi.getHours(locationId)).hours
    })
  async function saveHours(locationId: string, week: Hours[]) {
    const r = await save(() => practiceApi.setHours(locationId, week))
    if (r) hours.value = r.hours
    return r !== null
  }
  const loadClosures = () =>
    load('closures', async () => {
      closures.value = (await practiceApi.listClosures(isoDate(-365))).closures
    })
  async function addClosure(body: Omit<Closure, 'id'>) {
    const r = await save(() => practiceApi.createClosure(body))
    if (r) closures.value = [...closures.value, r.closure].sort((a, b) => a.starts_on.localeCompare(b.starts_on))
    return r !== null
  }
  async function removeClosure(id: string) {
    const r = await save(() => practiceApi.deleteClosure(id))
    if (r) closures.value = closures.value.filter(c => c.id !== id)
    return r !== null
  }

  return {
    state, profile, setupMissing, timeZones, slotLengths, locations, practitioners, members, services, hours, closures,
    saving, saveError,
    loadProfile, saveProfile, loadLocations, saveLocation, loadPractitioners, savePractitioner,
    loadServices, saveService, loadHours, saveHours, loadClosures, addClosure, removeClosure
  }
})

/** The local date `days` from today as YYYY-MM-DD. */
export function isoDate(days = 0, now = new Date()): string {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + days)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** "HH:MM" options on a slot grid, from 00:00 to 24:00. */
export function timeOptions(slot: number): string[] {
  const out: string[] = []
  for (let m = 0; m <= 24 * 60; m += slot) {
    out.push(`${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`)
  }
  return out
}

/** Duration options in whole slots, from one slot to 8 hours. */
export function durationOptions(slot: number): number[] {
  const out: number[] = []
  for (let m = slot; m <= 480; m += slot) out.push(m)
  return out
}

/** Dollars as typed ("85", "85.5", "") to cents, or null when empty; NaN when invalid. */
export function dollarsToCents(input: string): number | null {
  const s = input.trim().replace(/^\$/, '')
  if (s === '') return null
  if (!/^\d+(\.\d{1,2})?$/.test(s)) return NaN
  return Math.round(parseFloat(s) * 100)
}

export function centsToDollars(cents: number | null): string {
  return cents === null ? '' : (cents / 100).toFixed(2)
}
