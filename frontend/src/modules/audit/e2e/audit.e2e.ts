import { expect, test } from '@playwright/test'
import { apiAs, createAccounts, login, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('audit', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('an admin sees a connected system change in the audit log', async ({ page }) => {
    const api = await apiAs(baseURL(), owner)
    await api.call('PUT', '/api/x/connectors/account', { provider: 'builtin' })
    await api.dispose()
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Audit log', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/audit$/)
    const row = page.getByTestId('audit-row-connection_saved')
    await expect(row).toContainText('Connected system changed')
    await expect(row).toContainText(`Olivia Owner ${run}`)
    await expect(row).toContainText('provider: builtin')
  })

  test('reception has no audit log', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Audit log', exact: true })).toHaveCount(0)
  })
})
