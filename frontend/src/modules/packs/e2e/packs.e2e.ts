import { expect, test } from '@playwright/test'
import { createAccounts, login, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('industry packs', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('a manager previews the clinic pack and installs it; installing again changes nothing', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Industry packs', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/packs$/)
    await page.getByTestId('packs-preview-clinic').click()
    await expect(page.getByTestId('packs-preview')).toContainText('wl_clinic_appointment_reminder')
    await expect(page.getByTestId('packs-preview')).toContainText('Never give medical')

    await page.getByTestId('packs-install-clinic').click()
    await expect(page.getByTestId('packs-result')).toBeVisible()
    await expect(page.getByTestId('packs-installed-clinic')).toBeVisible()
    await expect(page.getByTestId('packs-result')).toContainText('Opted in to news')

    await page.getByTestId('packs-install-clinic').click()
    await expect(page.getByTestId('packs-item-segment-exists').first()).toBeVisible()
    await expect(page.getByTestId('packs-item-segment-created')).toHaveCount(0)
  })

  test('reception has no packs page', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Industry packs', exact: true })).toHaveCount(0)
  })
})
