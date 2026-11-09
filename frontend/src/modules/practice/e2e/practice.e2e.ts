import { expect, test } from '@playwright/test'
import { choose, createAccounts, login, type Account } from './setup'

// One journey per user story in wl-docs/modules/practice/requirements.md
// (test-plan.md, "End to end"). They run in order: each builds on the last.

const run = Date.now().toString(36)
const slug = `clinic-${run}`
const location = `Bondi ${run}`
let owner: Account
let reception: Account

test.describe.serial('practice setup', () => {
  test.beforeAll(async () => {
    const baseURL = test.info().project.use.baseURL ?? 'http://localhost:8080'
    ;({ owner, reception } = await createAccounts(baseURL, run))
  })

  test('owner sets up the practice profile and sees the booking link', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Practice profile' }).click()
    await expect(page).toHaveURL(/\/x\/practice\/profile$/)

    await page.getByTestId('profile-name').fill(`Harbour Medical ${run}`)
    await choose(page, 'profile-tz', 'Sydney')
    await page.getByTestId('profile-slug').fill(slug)
    await expect(page.getByTestId('profile-slug-preview')).toContainText(`/book/${slug}`)
    await page.getByTestId('profile-save').click()
    await expect(page.getByText('Saved').first()).toBeVisible()

    await page.reload()
    await expect(page.getByTestId('profile-slug')).toHaveValue(slug)
  })

  test('owner adds a location with weekday hours and a Good Friday closure', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/practice/locations')
    await page.getByTestId('location-add').click()
    await page.getByTestId('location-name').fill(location)
    await page.getByTestId('location-save').click()
    await expect(page.getByTestId('locations-table')).toContainText(location)

    await page.getByTestId('practice-tab-hours').click()
    await choose(page, 'hours-location', location)
    for (const day of [1, 2, 3, 4, 5]) {
      await page.getByTestId(`hours-open-${day}`).click()
      await choose(page, `hours-from-${day}`, '08:30')
      await choose(page, `hours-to-${day}`, '17:00')
    }
    await page.getByTestId('hours-save').click()
    await expect(page.getByText('Saved').first()).toBeVisible()

    await page.getByTestId('closure-reason').fill(`Good Friday ${run}`)
    await page.getByTestId('closure-from').fill('2027-03-26')
    await choose(page, 'closure-where', location)
    await page.getByTestId('closure-add').click()
    await expect(page.getByTestId('closures-list')).toContainText(`Good Friday ${run}`)

    // The week comes back as saved.
    await page.reload()
    await choose(page, 'hours-location', location)
    await expect(page.getByTestId('hours-from-1')).toHaveText('08:30')
    await expect(page.getByTestId('hours-to-5')).toHaveText('17:00')
    await expect(page.getByTestId('hours-open-6')).toHaveAttribute('aria-checked', 'false')
  })

  test('owner adds two practitioners, one linked to a login', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/practice/practitioners')

    await page.getByTestId('practitioner-add').click()
    await page.getByTestId('practitioner-name').fill(`Dr Ada ${run}`)
    await page.getByTestId('practitioner-role').fill('GP')
    await choose(page, 'practitioner-user', new RegExp(reception.email))
    await page.getByTestId(`practitioner-loc-${location}`).click()
    await page.getByTestId('practitioner-save').click()

    await page.getByTestId('practitioner-add').click()
    await page.getByTestId('practitioner-name').fill(`Nurse Bea ${run}`)
    await page.getByTestId('practitioner-role').fill('Nurse')
    await page.getByTestId(`practitioner-loc-${location}`).click()
    await page.getByTestId('practitioner-save').click()

    const table = page.getByTestId('practitioners-table')
    const ada = table.getByRole('row', { name: new RegExp(`Dr Ada ${run}`) })
    await expect(ada).toContainText(reception.full_name)
    await expect(ada).toContainText(location)
    const bea = table.getByRole('row', { name: new RegExp(`Nurse Bea ${run}`) })
    await expect(bea).toContainText('No login')
    await expect(bea).toContainText(location)
  })

  test('owner adds two services with roles and prices', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/practice/services')

    for (const s of [
      { name: `Standard consult ${run}`, duration: '15 min', price: '85', roles: 'GP' },
      { name: `Long consult ${run}`, duration: '30 min', price: '150.50', roles: 'GP, Nurse' }
    ]) {
      await page.getByTestId('service-add').click()
      await page.getByTestId('service-name').fill(s.name)
      await choose(page, 'service-duration', s.duration)
      await page.getByTestId('service-price').fill(s.price)
      await page.getByTestId('service-roles').fill(s.roles)
      await page.getByTestId('service-save').click()
      await expect(page.getByRole('dialog')).toBeHidden()
    }

    const table = page.getByTestId('services-table')
    await expect(table.getByRole('row', { name: new RegExp(`Standard consult ${run}`) })).toContainText('$85.00')
    const long = table.getByRole('row', { name: new RegExp(`Long consult ${run}`) })
    await expect(long).toContainText('30 min')
    await expect(long).toContainText('$150.50')
    await expect(long).toContainText('GP, Nurse')
  })

  test('reception sees all five pages read-only', async ({ page }) => {
    await login(page, reception)

    await page.goto('/x/practice/profile')
    await expect(page.getByTestId('profile-name')).toBeDisabled()
    await expect(page.getByTestId('profile-save')).toHaveCount(0)
    await expect(page.getByTestId('practice-readonly')).toBeVisible()
    for (const tab of ['profile', 'locations', 'practitioners', 'services', 'hours']) {
      await expect(page.getByTestId(`practice-tab-${tab}`)).toBeVisible()
    }

    await page.getByTestId('practice-tab-locations').click()
    await expect(page.getByTestId('locations-table')).toContainText(location)
    await expect(page.getByTestId('location-add')).toHaveCount(0)
    await expect(page.getByRole('switch')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Edit' })).toHaveCount(0)

    await page.getByTestId('practice-tab-practitioners').click()
    await expect(page.getByTestId('practitioners-table')).toContainText(`Dr Ada ${run}`)
    await expect(page.getByTestId('practitioner-add')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Edit' })).toHaveCount(0)

    await page.getByTestId('practice-tab-services').click()
    await expect(page.getByTestId('services-table')).toContainText(`Long consult ${run}`)
    await expect(page.getByTestId('service-add')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Edit' })).toHaveCount(0)

    await page.getByTestId('practice-tab-hours').click()
    await choose(page, 'hours-location', location)
    await expect(page.getByTestId('hours-from-1')).toHaveText('08:30')
    await expect(page.getByTestId('hours-open-1')).toBeDisabled()
    await expect(page.getByTestId('hours-save')).toHaveCount(0)
    await expect(page.getByTestId('practice-readonly')).toBeVisible()
    await expect(page.getByTestId('closures-list')).toContainText(`Good Friday ${run}`)
    await expect(page.getByTestId('closure-add')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Remove closure' })).toHaveCount(0)
  })
})
