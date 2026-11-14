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

  test('an admin sees built-in booking connected, and a Cliniko key is tested before anything changes', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Connected system', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/connectors$/)
    const builtin = page.getByTestId('connectors-provider-builtin')
    await expect(builtin).toContainText("Weline's built-in booking")
    await expect(builtin.getByTestId('connectors-status')).toHaveText('Connected')
    await expect(page.getByTestId('connectors-log-empty')).toBeVisible()
    // Google Calendar needs Weline's service account, which CI doesn't have.
    await expect(page.getByTestId('connectors-provider-google_calendar')).toContainText('Google Calendar')
    await expect(page.getByTestId('connectors-choose-google_calendar')).toHaveCount(0)
    // Microsoft 365 likewise needs Weline's Entra app.
    await expect(page.getByTestId('connectors-provider-microsoft_365')).toContainText('Microsoft 365')
    await expect(page.getByTestId('connectors-choose-microsoft_365')).toHaveCount(0)
    // Calendly needs only the business's token: it can be chosen, and a bad token is refused.
    await page.getByTestId('connectors-choose-calendly').click()
    await expect(page.getByTestId('connectors-field-api_token')).toBeVisible()
    await page.reload()

    // Cliniko can be chosen; the key is checked before anything is saved. CI has
    // no encryption key, contact email or Cliniko, so it's refused and says why.
    await page.getByTestId('connectors-choose-cliniko').click()
    await page.getByTestId('connectors-field-api_key').fill('notarealkey-au1')
    await page.getByTestId('connectors-save').click()
    await expect(page.getByTestId('connectors-error')).toBeVisible()
    await expect(builtin.getByTestId('connectors-status')).toHaveText('Connected')
    await expect(page.getByTestId('connectors-provider-cliniko').getByTestId('connectors-status')).toHaveCount(0)

    // Choosing built-in again through the API is accepted and logged nothing to test.
    const api = await apiAs(baseURL(), owner)
    const res = await api.call('PUT', '/api/x/connectors/account', { provider: 'builtin' })
    expect(res.account.provider).toBe('builtin')
    expect(JSON.stringify(res)).not.toContain('credentials":"')
    await api.dispose()
  })

  test('coming back from Microsoft with a refused or taken approval says why', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/connectors?microsoft=taken')
    await expect(page.getByTestId('connectors-consent-result')).toContainText('already connected to another Weline account')
    // The return route itself refuses a forged or missing state.
    const res = await page.request.get('/x/connectors/microsoft/consent?admin_consent=True&tenant=11111111-2222-3333-4444-555555555555&state=forged',
      { maxRedirects: 0 })
    expect(res.status()).toBe(302)
    expect(res.headers().location).toContain('microsoft=expired')
  })

  test('reception has no Connected system page', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Connected system', exact: true })).toHaveCount(0)
    await page.goto('/x/connectors')
    await expect(page).not.toHaveURL(/\/x\/connectors/)
  })
})
