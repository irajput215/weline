import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'
import { practiceApi, type Location, type Practitioner, type Profile, type Service } from '@/modules/practice/api'
import {
  bookingApi,
  type Appointment,
  type Availability,
  type Block,
  type HistoryEntry,
  type ListQuery,
  type NewAppointment,
  type Readiness,
  type Settings,
  type Slot,
  type SlotQuery,
  type Status
} from '../api'

/** What a booking page should show for one resource. */
export type LoadState = 'idle' | 'loading' | 'ready' | 'error' | 'forbidden'
type Resource = 'clinic' | 'appointments' | 'availability' | 'blocks' | 'settings'

function statusOf(err: unknown): number | undefined {
  return (err as { response?: { status?: number } })?.response?.status
}

function messageOf(err: unknown): string {
  return (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
}

/** Bookings module: one store for its three pages. */
export const useBookingStore = defineStore('wl-booking', () => {
  const state = reactive<Record<Resource, LoadState>>({
    clinic: 'idle',
    appointments: 'idle',
    availability: 'idle',
    blocks: 'idle',
    settings: 'idle'
  })

  // Clinic data from the practice module.
  const profile = ref<Profile | null>(null)
  const locations = ref<Location[]>([])
  const practitioners = ref<Practitioner[]>([])
  const services = ref<Service[]>([])

  const appointments = ref<Appointment[]>([])
  const availability = ref<Availability[]>([])
  const blocks = ref<Block[]>([])
  const settings = ref<Settings | null>(null)
  const ready = ref<Readiness | null>(null)

  const slots = ref<Slot[]>([])
  const slotsLoading = ref(false)
  const slotsError = ref<string | null>(null)

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

  const loadClinic = () =>
    load('clinic', async () => {
      const [p, l, pr, s] = await Promise.all([
        practiceApi.getProfile(),
        practiceApi.listLocations(false),
        practiceApi.listPractitioners(false),
        practiceApi.listServices(false)
      ])
      profile.value = p.profile
      locations.value = l.locations
      practitioners.value = pr.practitioners
      services.value = s.services
    })

  const loadAppointments = (q: ListQuery) =>
    load('appointments', async () => {
      appointments.value = (await bookingApi.listAppointments(q)).appointments
    })

  function replace(a: Appointment) {
    const i = appointments.value.findIndex(x => x.id === a.id)
    if (i >= 0) appointments.value.splice(i, 1, a)
  }

  let slotsSeq = 0 // only the latest search may set the slots
  async function searchSlots(q: SlotQuery) {
    const mine = ++slotsSeq
    slotsLoading.value = true
    slotsError.value = null
    try {
      const found = (await bookingApi.slots(q)).slots
      if (mine === slotsSeq) slots.value = found
    } catch (err) {
      if (mine === slotsSeq) {
        slots.value = []
        slotsError.value = messageOf(err) || 'error'
      }
    } finally {
      if (mine === slotsSeq) slotsLoading.value = false
    }
  }

  /** Books; returns the appointment, or null with saveError set. */
  async function book(body: NewAppointment): Promise<Appointment | null> {
    const r = await save(() => bookingApi.createAppointment(body))
    return r?.appointment ?? null
  }

  async function setStatus(id: string, status: Status, reason = '') {
    const r = await save(() => bookingApi.setStatus(id, status, reason))
    if (r) replace(r.appointment)
    return r !== null
  }

  async function move(id: string, body: Parameters<typeof bookingApi.moveAppointment>[1]) {
    const r = await save(() => bookingApi.moveAppointment(id, body))
    if (r) replace(r.appointment)
    return r !== null
  }

  async function history(id: string): Promise<HistoryEntry[]> {
    try {
      return (await bookingApi.getAppointment(id)).history
    } catch {
      return []
    }
  }

  const loadAvailability = (practitionerId: string) =>
    load('availability', async () => {
      availability.value = (await bookingApi.getAvailability(practitionerId)).blocks
    })

  /** Saves the week; returns warnings, or null on failure. */
  async function saveAvailability(practitionerId: string, week: Availability[]): Promise<string[] | null> {
    const r = await save(() => bookingApi.setAvailability(practitionerId, week))
    if (!r) return null
    availability.value = r.blocks
    return r.warnings
  }

  const loadBlocks = (practitionerId?: string) =>
    load('blocks', async () => {
      blocks.value = (await bookingApi.listBlocks(practitionerId)).blocks
    })

  /** Adds a block; returns appointments inside it, or null on failure. */
  async function addBlock(body: Parameters<typeof bookingApi.createBlock>[0]): Promise<Appointment[] | null> {
    const r = await save(() => bookingApi.createBlock(body))
    if (!r) return null
    blocks.value = [...blocks.value, r.block].sort((a, b) => a.starts_at.localeCompare(b.starts_at))
    return r.clashes
  }

  async function removeBlock(id: string) {
    const r = await save(() => bookingApi.deleteBlock(id))
    if (r) blocks.value = blocks.value.filter(b => b.id !== id)
    return r !== null
  }

  const loadSettings = () =>
    load('settings', async () => {
      const r = await bookingApi.getSettings()
      settings.value = r.settings
      ready.value = r.ready
    })

  async function saveSettings(body: Settings) {
    const r = await save(() => bookingApi.saveSettings(body))
    if (r) {
      settings.value = r.settings
      ready.value = r.ready
    }
    return r !== null
  }

  return {
    state, profile, locations, practitioners, services, appointments, availability, blocks, settings, ready,
    slots, slotsLoading, slotsError, saving, saveError,
    loadClinic, loadAppointments, searchSlots, book, setStatus, move, history,
    loadAvailability, saveAvailability, loadBlocks, addBlock, removeBlock, loadSettings, saveSettings
  }
})

/** Today's date (YYYY-MM-DD) in a time zone. */
export function todayIn(timeZone: string, now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
}

/** A date string moved by n days. */
export function addDays(date: string, n: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

/** "HH:MM" options on a slot grid from 00:00 to 24:00. */
export function gridTimes(slot: number): string[] {
  const out: string[] = []
  for (let m = 0; m <= 24 * 60; m += slot) {
    out.push(`${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`)
  }
  return out
}

/** Overlapping blocks on the same weekday (any location), as pairs of indexes. */
export function overlaps(week: Availability[]): [number, number][] {
  const out: [number, number][] = []
  for (let i = 0; i < week.length; i++) {
    for (let j = i + 1; j < week.length; j++) {
      const a = week[i], b = week[j]
      if (a.weekday === b.weekday && a.starts_at < b.ends_at && b.starts_at < a.ends_at) out.push([i, j])
    }
  }
  return out
}

/** Instant (ISO) of a local wall-clock time in a time zone, e.g. ("2027-03-08", "09:00", "Australia/Sydney"). */
export function instantOf(date: string, time: string, timeZone: string): string {
  // Start from the time as if it were UTC, then correct by the zone's offset at that moment.
  const asUTC = new Date(`${date}T${time}:00Z`)
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
  }).formatToParts(asUTC)
  const get = (t: string) => Number(parts.find(p => p.type === t)?.value)
  const shown = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'))
  return new Date(asUTC.getTime() - (shown - asUTC.getTime())).toISOString()
}

export const statusTone: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  BOOKED: 'outline',
  CONFIRMED: 'secondary',
  ARRIVED: 'default',
  COMPLETED: 'secondary',
  CANCELLED: 'destructive',
  NO_SHOW: 'destructive'
}
