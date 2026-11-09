import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const api = vi.hoisted(() => ({
  getProfile: vi.fn(),
  saveProfile: vi.fn(),
  listLocations: vi.fn(),
  createLocation: vi.fn(),
  updateLocation: vi.fn(),
  createService: vi.fn(),
  setHours: vi.fn(),
  listClosures: vi.fn(),
  createClosure: vi.fn(),
  deleteClosure: vi.fn()
}))
vi.mock('../api', () => ({ practiceApi: api }))

import { centsToDollars, dollarsToCents, durationOptions, timeOptions, usePracticeStore } from './practice'

const httpError = (status: number, message?: string) => ({ response: { status, data: { message } } })
const loc = (id: string, name: string) => ({ id, name, address: '', phone: '', is_active: true })
const profile = {
  id: 'p1', name: 'Harbour Medical', time_zone: 'Australia/Sydney', booking_slug: 'harbour', slot_minutes: 15,
  abn: '', phone: '', email: '', website: ''
}

describe('practice store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(api).forEach(fn => fn.mockReset())
  })

  it('loads the profile and what setup is missing', async () => {
    api.getProfile.mockResolvedValue({
      profile: null, setup: { missing: ['profile', 'locations'] }, time_zones: ['Australia/Sydney'], slot_lengths: [15]
    })
    const store = usePracticeStore()
    const pending = store.loadProfile()
    expect(store.state.profile).toBe('loading')
    await pending
    expect(store.state.profile).toBe('ready')
    expect(store.profile).toBeNull()
    expect(store.setupMissing).toEqual(['profile', 'locations'])
    expect(store.timeZones).toEqual(['Australia/Sydney'])
  })

  it('shows forbidden on 403 and error otherwise', async () => {
    const store = usePracticeStore()
    api.listLocations.mockRejectedValueOnce(httpError(403))
    await store.loadLocations()
    expect(store.state.locations).toBe('forbidden')

    api.listLocations.mockRejectedValueOnce(httpError(500))
    await store.loadLocations()
    expect(store.state.locations).toBe('error')
  })

  it('saves the profile and clears it from the setup list', async () => {
    api.getProfile.mockResolvedValue({ profile: null, setup: { missing: ['profile', 'hours'] }, time_zones: [], slot_lengths: [] })
    api.saveProfile.mockResolvedValue({ profile })
    const store = usePracticeStore()
    await store.loadProfile()

    expect(await store.saveProfile({ ...profile })).toBe(true)
    expect(store.profile?.name).toBe('Harbour Medical')
    expect(store.setupMissing).toEqual(['hours'])
  })

  it('adds a new location and replaces an edited one', async () => {
    api.listLocations.mockResolvedValue({ locations: [loc('1', 'Bondi')] })
    api.createLocation.mockResolvedValue({ location: loc('2', 'Manly') })
    api.updateLocation.mockResolvedValue({ location: { ...loc('1', 'Bondi'), is_active: false } })
    const store = usePracticeStore()
    await store.loadLocations()

    expect(await store.saveLocation(null, { name: 'Manly' })).toBe(true)
    expect(await store.saveLocation('1', { is_active: false })).toBe(true)
    expect(api.updateLocation).toHaveBeenCalledWith('1', { is_active: false })
    expect(store.locations.map(l => [l.name, l.is_active])).toEqual([['Bondi', false], ['Manly', true]])
  })

  it('keeps the server message when a save is refused', async () => {
    const store = usePracticeStore()
    api.createLocation.mockRejectedValueOnce(httpError(409, 'a location named Bondi already exists'))
    expect(await store.saveLocation(null, { name: 'Bondi' })).toBe(false)
    expect(store.saveError).toBe('a location named Bondi already exists')
    expect(store.saving).toBe(false)

    api.createLocation.mockRejectedValueOnce(httpError(403))
    expect(await store.saveLocation(null, { name: 'Bondi' })).toBe(false)
    expect(store.saveError).toBe('forbidden')
    expect(store.locations).toEqual([])
  })

  it('returns service warnings, or null when the save fails', async () => {
    const store = usePracticeStore()
    api.createService.mockResolvedValueOnce({
      service: { id: 's1', name: 'Standard consult', duration_minutes: 15, price_cents: 8500, allowed_roles: ['GP'], is_active: true },
      warnings: ['No practitioner has the role GP yet']
    })
    expect(await store.saveService(null, { name: 'Standard consult' })).toEqual(['No practitioner has the role GP yet'])
    expect(store.services).toHaveLength(1)

    api.createService.mockRejectedValueOnce(httpError(400, 'duration must be a whole number of 15-minute slots'))
    expect(await store.saveService(null, { name: 'x' })).toBeNull()
    expect(store.saveError).toContain('15-minute')
  })

  it('saves hours and keeps closures sorted', async () => {
    const week = [{ weekday: 1, opens_at: '08:30', closes_at: '17:00' }]
    api.setHours.mockResolvedValue({ hours: week })
    const store = usePracticeStore()
    expect(await store.saveHours('1', week)).toBe(true)
    expect(api.setHours).toHaveBeenCalledWith('1', week)
    expect(store.hours).toEqual(week)

    api.listClosures.mockResolvedValue({ closures: [{ id: 'c2', location_id: null, starts_on: '2027-12-25', ends_on: '2027-12-25', reason: 'Christmas' }] })
    api.createClosure.mockResolvedValue({ closure: { id: 'c1', location_id: null, starts_on: '2027-03-26', ends_on: '2027-03-26', reason: 'Good Friday' } })
    api.deleteClosure.mockResolvedValue({ deleted: true })
    await store.loadClosures()
    await store.addClosure({ location_id: null, starts_on: '2027-03-26', ends_on: '2027-03-26', reason: 'Good Friday' })
    expect(store.closures.map(c => c.id)).toEqual(['c1', 'c2'])
    await store.removeClosure('c2')
    expect(store.closures.map(c => c.id)).toEqual(['c1'])
  })
})

describe('practice helpers', () => {
  it('builds time options on the slot grid', () => {
    const t = timeOptions(30)
    expect(t[0]).toBe('00:00')
    expect(t).toContain('08:30')
    expect(t.at(-1)).toBe('24:00')
    expect(t).toHaveLength(49)
  })

  it('builds durations in whole slots up to 8 hours', () => {
    expect(durationOptions(60)).toEqual([60, 120, 180, 240, 300, 360, 420, 480])
    expect(durationOptions(15)[0]).toBe(15)
  })

  it('converts dollars and cents', () => {
    expect(dollarsToCents('')).toBeNull()
    expect(dollarsToCents('85')).toBe(8500)
    expect(dollarsToCents('$85.5')).toBe(8550)
    expect(dollarsToCents('85.505')).toBeNaN()
    expect(dollarsToCents('abc')).toBeNaN()
    expect(centsToDollars(8550)).toBe('85.50')
    expect(centsToDollars(null)).toBe('')
  })
})
