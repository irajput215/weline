import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const b = vi.hoisted(() => ({
  slots: vi.fn(),
  listAppointments: vi.fn(),
  getAppointment: vi.fn(),
  createAppointment: vi.fn(),
  moveAppointment: vi.fn(),
  setStatus: vi.fn(),
  getAvailability: vi.fn(),
  setAvailability: vi.fn(),
  listBlocks: vi.fn(),
  createBlock: vi.fn(),
  deleteBlock: vi.fn(),
  getSettings: vi.fn(),
  saveSettings: vi.fn()
}))
const p = vi.hoisted(() => ({
  getProfile: vi.fn(),
  listLocations: vi.fn(),
  listPractitioners: vi.fn(),
  listServices: vi.fn()
}))
vi.mock('../api', () => ({ bookingApi: b }))
vi.mock('@/modules/practice/api', () => ({ practiceApi: p }))

import { addDays, gridTimes, instantOf, overlaps, todayIn, useBookingStore } from './booking'
import type { Appointment } from '../api'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })
const appt = (id: string, status: Appointment['status'] = 'BOOKED') =>
  ({ id, status, reference: 'ABC234', allowed_next: [] }) as unknown as Appointment

describe('booking store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(b).forEach(fn => fn.mockReset())
    Object.values(p).forEach(fn => fn.mockReset())
  })

  it('loads the clinic from the practice module', async () => {
    p.getProfile.mockResolvedValue({ profile: { time_zone: 'Australia/Perth', slot_minutes: 15 } })
    p.listLocations.mockResolvedValue({ locations: [{ id: 'l1' }] })
    p.listPractitioners.mockResolvedValue({ practitioners: [{ id: 'p1' }] })
    p.listServices.mockResolvedValue({ services: [{ id: 's1' }] })
    const store = useBookingStore()
    await store.loadClinic()
    expect(store.state.clinic).toBe('ready')
    expect(p.listLocations).toHaveBeenCalledWith(false)
    expect(store.profile?.time_zone).toBe('Australia/Perth')
    expect(store.practitioners).toHaveLength(1)
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = useBookingStore()
    b.listAppointments.mockRejectedValueOnce(httpError(403))
    await store.loadAppointments({ from: '2027-03-08' })
    expect(store.state.appointments).toBe('forbidden')
    b.listAppointments.mockRejectedValueOnce(httpError(500))
    await store.loadAppointments({ from: '2027-03-08' })
    expect(store.state.appointments).toBe('error')
  })

  it('keeps the clash message when booking fails, and the appointment when it works', async () => {
    const store = useBookingStore()
    b.createAppointment.mockRejectedValueOnce(httpError(409, 'Dr Ada already has an appointment 09:00–09:15 on Mon 8 Mar'))
    expect(await store.book({ contact_id: 'c', service_id: 's', location_id: 'l', practitioner_id: 'p', starts_at: 'x' })).toBeNull()
    expect(store.saveError).toContain('already has an appointment')
    b.createAppointment.mockResolvedValueOnce({ appointment: appt('a1'), confirmation: 'Booked' })
    expect((await store.book({ contact_id: 'c', service_id: 's', location_id: 'l', practitioner_id: 'p', starts_at: 'x' }))?.id).toBe('a1')
    expect(store.saveError).toBeNull()
  })

  it('replaces an appointment in the list after a status change', async () => {
    b.listAppointments.mockResolvedValue({ appointments: [appt('a1'), appt('a2')] })
    b.setStatus.mockResolvedValue({ appointment: appt('a2', 'ARRIVED') })
    const store = useBookingStore()
    await store.loadAppointments({ from: '2027-03-08' })
    expect(await store.setStatus('a2', 'ARRIVED')).toBe(true)
    expect(b.setStatus).toHaveBeenCalledWith('a2', 'ARRIVED', '')
    expect(store.appointments.map(a => a.status)).toEqual(['BOOKED', 'ARRIVED'])

    b.setStatus.mockRejectedValueOnce(httpError(403))
    expect(await store.setStatus('a1', 'CANCELLED', 'x')).toBe(false)
    expect(store.saveError).toBe('forbidden')
  })

  it('records slot search errors without throwing', async () => {
    const store = useBookingStore()
    b.slots.mockRejectedValueOnce(httpError(400, 'Dr Ada (GP) doesn\'t provide Dressing'))
    await store.searchSlots({ service_id: 's', location_id: 'l', from: '2027-03-08' })
    expect(store.slots).toEqual([])
    expect(store.slotsError).toContain('provide Dressing')
    expect(store.slotsLoading).toBe(false)
  })

  it('keeps only the latest slot search when answers arrive out of order', async () => {
    const store = useBookingStore()
    let first!: (v: unknown) => void
    b.slots.mockImplementationOnce(() => new Promise(r => (first = r)))
    b.slots.mockResolvedValueOnce({ slots: [{ starts_at: 'new' }] })
    const a = store.searchSlots({ service_id: 's', location_id: 'l', from: '2027-03-08' })
    await store.searchSlots({ service_id: 's', location_id: 'l', from: '2027-03-09' })
    first({ slots: [{ starts_at: 'stale' }] })
    await a
    expect(store.slots.map(s => s.starts_at)).toEqual(['new'])
    expect(store.slotsLoading).toBe(false)
  })

  it('returns availability warnings, and keeps blocks sorted', async () => {
    const store = useBookingStore()
    b.setAvailability.mockResolvedValueOnce({ blocks: [], warnings: ['Sat 09:00–12:00 at Bondi is outside opening hours'] })
    expect(await store.saveAvailability('p1', [])).toEqual(['Sat 09:00–12:00 at Bondi is outside opening hours'])
    b.listBlocks.mockResolvedValue({ blocks: [{ id: 'b2', starts_at: '2027-03-10T00:00:00Z' }] })
    await store.loadBlocks('p1')
    b.createBlock.mockResolvedValue({ block: { id: 'b1', starts_at: '2027-03-09T00:00:00Z' }, clashes: [appt('a1')] })
    expect(await store.addBlock({ practitioner_id: 'p1', starts_at: 'x', ends_at: 'y', reason: 'Leave' })).toHaveLength(1)
    expect(store.blocks.map(x => x.id)).toEqual(['b1', 'b2'])
    b.deleteBlock.mockResolvedValue({ deleted: true })
    await store.removeBlock('b1')
    expect(store.blocks.map(x => x.id)).toEqual(['b2'])
  })
})

describe('booking helpers', () => {
  it('finds today in the clinic time zone', () => {
    const now = new Date('2027-03-07T14:30:00Z') // Monday 01:30 in Sydney, still Sunday in Perth
    expect(todayIn('Australia/Sydney', now)).toBe('2027-03-08')
    expect(todayIn('Australia/Perth', now)).toBe('2027-03-07')
  })

  it('moves dates by days', () => {
    expect(addDays('2027-02-28', 1)).toBe('2027-03-01')
    expect(addDays('2027-03-01', -1)).toBe('2027-02-28')
  })

  it('turns clinic wall-clock times into instants, across daylight saving', () => {
    expect(instantOf('2027-03-08', '09:00', 'Australia/Sydney')).toBe('2027-03-07T22:00:00.000Z') // AEDT +11
    expect(instantOf('2027-06-07', '09:00', 'Australia/Sydney')).toBe('2027-06-06T23:00:00.000Z') // AEST +10
    expect(instantOf('2027-03-08', '09:00', 'Australia/Perth')).toBe('2027-03-08T01:00:00.000Z')
  })

  it('builds the grid and spots overlapping hours', () => {
    expect(gridTimes(30)).toHaveLength(49)
    expect(gridTimes(60).at(-1)).toBe('24:00')
    const week = [
      { location_id: 'a', weekday: 1, starts_at: '08:00', ends_at: '12:00' },
      { location_id: 'b', weekday: 1, starts_at: '11:00', ends_at: '13:00' },
      { location_id: 'a', weekday: 1, starts_at: '13:00', ends_at: '17:00' },
      { location_id: 'a', weekday: 2, starts_at: '08:00', ends_at: '12:00' }
    ]
    expect(overlaps(week)).toEqual([[0, 1]])
  })
})
