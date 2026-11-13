import { expect, test } from '@playwright/test'
import { apiAs, createAccounts, login, type Account } from '../../practice/e2e/setup'

// Journeys from wl-docs/modules/connectors/test-plan.md ("End to end"). The
// Cliniko provider comes in 3.2; until then built-in is the only choice.

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('connectors', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('an admin sees built-in booking connected and Cliniko coming soon', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Connected system', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/connectors$/)
    const builtin = page.getByTestId('connectors-provider-builtin')
    await expect(builtin).toContainText("Weline's built-in booking")
    await expect(builtin.getByTestId('connectors-status')).toHaveText('Connected')
    const cliniko = page.getByTestId('connectors-provider-cliniko')
    await expect(cliniko).toContainText('Coming soon')
    await expect(page.getByTestId('connectors-choose-cliniko')).toHaveCount(0)
    await expect(page.getByTestId('connectors-log-empty')).toBeVisible()

    // Choosing built-in again through the API is accepted and logged nothing to test.
    const api = await apiAs(baseURL(), owner)
    const res = await api.call('PUT', '/api/x/connectors/account', { provider: 'builtin' })
    expect(res.account.provider).toBe('builtin')
    expect(JSON.stringify(res)).not.toContain('credentials":"')
    await api.dispose()
  })

  test('reception has no Connected system page', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Connected system', exact: true })).toHaveCount(0)
    await page.goto('/x/connectors')
    await expect(page).not.toHaveURL(/\/x\/connectors/)
  })
})
