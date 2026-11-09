import { expect, test } from '@playwright/test'
import { apiAs, choose, createAccounts, login, type Account } from '../../practice/e2e/setup'

// One journey per user story in wl-docs/modules/booking/requirements.md
// (test-plan.md, "End to end"). They run in order: each builds on the last.
// The WhatsApp story is covered by a Go handler test that replays the flow's
// api_call requests (there's no WhatsApp in CI).

const run = Date.now().toString(36)
const TZ = 'Australia/Sydney'
const slug = `bk-${run}`
const site = `Booking site ${run}`
const service = `Booking consult ${run}`
const bo = `Dr Bo ${run}`
const cy = `Dr Cy ${run}`
const patient = `Pat ${run}`
const mobile = `04${String(Date.now()).slice(-8)}`

/** A local date in the clinic's zone, n days from today. */
function localDate(n: number): string {
  const d = new Date(Date.now() + n * 86_400_000)
  return new Intl.DateTimeFormat('en-CA', { timeZone: TZ, year: 'numeric', month: '2-digit', day: '2-digit' }).format(d)
}
const tomorrow = localDate(1)
const weekday = ((new Date(`${tomorrow}T12:00:00Z`).getUTCDay() + 6) % 7) + 1 // ISO 1 = Monday

let owner: Account
let reception: Account

test.describe.serial('booking', () => {
  test.beforeAll(async () => {
    const baseURL = test.info().project.use.baseURL ?? 'http://localhost:8080'
    ;({ owner, reception } = await createAccounts(baseURL, run))
    const api = await apiAs(baseURL, owner)
    await api.call('PUT', '/api/x/practice/profile', { name: `Booking Clinic ${run}`, time_zone: TZ, booking_slug: slug, slot_minutes: 15 })
    const loc = (await api.call('POST', '/api/x/practice/locations', { name: site })).location.id
    const week = [1, 2, 3, 4, 5, 6, 7].map(d => ({ weekday: d, opens_at: '08:00', closes_at: '18:00' }))
    await api.call('PUT', `/api/x/practice/locations/${loc}/hours`, { hours: week })
    const pr = async (name: string) => (await api.call('POST', '/api/x/practice/practitioners', { display_name: name, role: 'GP', location_ids: [loc] })).practitioner.id
    await pr(bo)
    const cyId = await pr(cy)
    await api.call('POST', '/api/x/practice/services', { name: service, duration_minutes: 15, price_cents: 8500, allowed_roles: ['GP'] })
    await api.call('PUT', `/api/x/booking/practitioners/${cyId}/availability`, {
      blocks: [{ location_id: loc, weekday, starts_at: '09:00', ends_at: '12:00' }]
    })
    // New patients become Weline contacts on the clinic's WhatsApp account.
    await api.call('POST', '/api/accounts', { name: `E2E WA ${run}`, phone_id: `p${run}`, business_id: `b${run}`, access_token: 'e2e-not-a-real-token' })
    await api.dispose()
  })

  test('manager sets weekly hours and time off; blocked time shows no slots', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Availability' }).click()
    await expect(page).toHaveURL(/\/x\/booking\/availability$/)
    await choose(page, 'availability-practitioner', `${bo} (GP)`)

    await page.getByTestId('avail-add').click()
    await choose(page, 'avail-day-0', ['', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'][weekday])
    await choose(page, 'avail-from-0', '09:00')
    await choose(page, 'avail-to-0', '12:00')
    await page.getByTestId('avail-save').click()
    await expect(page.getByText('Saved').first()).toBeVisible()

    await page.getByTestId('block-reason').fill(`Leave ${run}`)
    await page.getByTestId('block-day').fill(tomorrow)
    await choose(page, 'block-from', '10:00')
    await choose(page, 'block-to', '11:00')
    await page.getByTestId('block-add').click()
    await expect(page.getByTestId('blocks-list')).toContainText(`Leave ${run}`)

    await page.getByTestId('booking-tab-appointments').click()
    await page.getByTestId('appointment-new').click()
    await choose(page, 'new-service', new RegExp(`^${service}`))
    await choose(page, 'new-location', site)
    await choose(page, 'new-practitioner', `${bo} (GP)`)
    await page.getByTestId('new-day').fill(tomorrow)
    const times = page.getByTestId('new-times')
    await expect(times.getByRole('button', { name: '09:00', exact: true })).toBeVisible()
    await expect(times.getByRole('button', { name: '09:45', exact: true })).toBeVisible()
    await expect(times.getByRole('button', { name: '10:00', exact: true })).toHaveCount(0)
    await expect(times.getByRole('button', { name: '10:45', exact: true })).toHaveCount(0)
    await expect(times.getByRole('button', { name: '11:00', exact: true })).toBeVisible()
  })

  test('reception books a new patient with anyone; the time is then taken', async ({ page }) => {
    await login(page, reception)
    await page.goto('/x/booking/appointments')
    await page.getByTestId('appointments-day').fill(tomorrow)
    await page.getByTestId('appointment-new').click()
    await choose(page, 'new-service', new RegExp(`^${service}`))
    await choose(page, 'new-location', site)
    await page.getByTestId('new-day').fill(tomorrow)
    await page.getByTestId('new-times').getByRole('button', { name: '09:00', exact: true }).click()
    await page.getByTestId('new-patient-add').click()
    await page.getByTestId('new-patient-name').fill(patient)
    await page.getByTestId('new-patient-mobile').fill(mobile)
    await page.getByTestId('new-patient-save').click()
    await expect(page.getByTestId('new-patient-chosen')).toContainText(patient)
    await page.getByTestId('new-book').click()
    await expect(page.getByText(/Booked\. Reference/).first()).toBeVisible()

    const table = page.getByTestId('appointments-table')
    const row = table.getByRole('row', { name: new RegExp(patient) })
    await expect(row).toContainText('09:00')
    await expect(row).toContainText(bo) // "anyone" books the first free practitioner by name
    await expect(row).toContainText('Booked')

    await page.getByTestId('appointment-new').click()
    await choose(page, 'new-service', new RegExp(`^${service}`))
    await choose(page, 'new-location', site)
    await choose(page, 'new-practitioner', `${bo} (GP)`)
    await page.getByTestId('new-day').fill(tomorrow)
    await expect(page.getByTestId('new-times').getByRole('button', { name: '09:15', exact: true })).toBeVisible()
    await expect(page.getByTestId('new-times').getByRole('button', { name: '09:00', exact: true })).toHaveCount(0)
  })

  test('reception moves, completes and cancels; history records each change', async ({ page }) => {
    await login(page, reception)
    await page.goto('/x/booking/appointments')
    await page.getByTestId('appointments-day').fill(tomorrow)
    const row = () => page.getByTestId('appointments-table').getByRole('row', { name: new RegExp(patient) })

    await row().getByTestId('action-move').click()
    await page.getByTestId('move-times').getByRole('button', { name: '09:30', exact: true }).click()
    await page.getByTestId('move-confirm').click()
    await expect(row()).toContainText('09:30')

    await row().getByTestId('action-ARRIVED').click()
    await expect(row().getByTestId('appointment-status')).toHaveText('Arrived')
    await row().getByTestId('action-COMPLETED').click()
    await expect(row().getByTestId('appointment-status')).toHaveText('Completed')
    await expect(row().getByTestId('action-CANCELLED')).toHaveCount(0)

    await row().getByTestId('action-history').click()
    const history = page.getByTestId('history-list')
    await expect(history.locator('li')).toHaveCount(4)
    await expect(history).toContainText('Booked (Staff)')
    await expect(history).toContainText('Changed time')
    await expect(history).toContainText('Arrived → Completed')
    await page.keyboard.press('Escape')

    // A second booking (Dr Cy, 11:30) to cancel.
    await page.getByTestId('appointment-new').click()
    await choose(page, 'new-service', new RegExp(`^${service}`))
    await choose(page, 'new-location', site)
    await choose(page, 'new-practitioner', `${cy} (GP)`)
    await page.getByTestId('new-day').fill(tomorrow)
    await page.getByTestId('new-times').getByRole('button', { name: '11:30', exact: true }).click()
    await page.getByTestId('new-patient-search').fill(patient)
    await page.getByRole('button', { name: new RegExp(`^${patient}`) }).click()
    await page.getByTestId('new-book').click()
    await expect(page.getByText(/Booked\. Reference/).first()).toBeVisible()

    const second = page.getByTestId('appointments-table').getByRole('row', { name: new RegExp(`11:30.*${patient}`) })
    await second.getByTestId('action-CANCELLED').click()
    await page.getByTestId('cancel-reason').fill('Patient unwell')
    await page.getByTestId('cancel-confirm').click()
    await expect(second.getByTestId('appointment-status')).toHaveText('Cancelled')

    await page.getByTestId('appointment-new').click()
    await choose(page, 'new-service', new RegExp(`^${service}`))
    await choose(page, 'new-location', site)
    await choose(page, 'new-practitioner', `${cy} (GP)`)
    await page.getByTestId('new-day').fill(tomorrow)
    await expect(page.getByTestId('new-times').getByRole('button', { name: '11:30', exact: true })).toBeVisible()
  })

  test('patient books on the public page without logging in', async ({ page }) => {
    test.skip(!process.env.WL_E2E_CAPTCHA, 'needs Turnstile test keys and internet (set WL_E2E_CAPTCHA=1); runs in CI')
    await page.context().clearCookies()
    await page.goto(`/book/${slug}`)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(`Booking Clinic ${run}`)
    await page.getByTestId('book-service').selectOption({ label: `${service} · 15 min · $85.00` })
    await page.getByTestId('book-location').selectOption({ label: site })
    await page.getByTestId('book-practitioner').selectOption({ label: `${cy} (GP)` })
    await page.getByTestId('book-day').fill(tomorrow)
    await page.getByTestId('book-day').dispatchEvent('change')
    await page.getByTestId('book-times').getByRole('button', { name: '10:00', exact: true }).click()
    await page.getByTestId('book-name').fill(`Web ${patient}`)
    await page.getByTestId('book-mobile').fill(`04${String(Date.now() + 7).slice(-8)}`)
    await page.getByTestId('book-notice').check()
    // Turnstile's test site key passes on its own; wait for its token.
    await page.waitForFunction(() => {
      const el = document.querySelector('[name="cf-turnstile-response"]') as HTMLInputElement | null
      return !!el && el.value.length > 0
    }, undefined, { timeout: 30_000 })
    await page.getByTestId('book-submit').click()
    await expect(page.getByTestId('book-done')).toBeVisible()
    await expect(page.getByTestId('book-ref')).toHaveText(/^[A-Z2-9]{6}$/)

    await login(page, reception)
    await page.goto('/x/booking/appointments')
    await page.getByTestId('appointments-day').fill(tomorrow)
    const row = page.getByTestId('appointments-table').getByRole('row', { name: new RegExp(`Web ${patient}`) })
    await expect(row).toContainText('10:00')
    await expect(row).toContainText('Online')
  })
})
