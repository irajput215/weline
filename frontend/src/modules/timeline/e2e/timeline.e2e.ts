import { expect, test } from '@playwright/test'
import { apiAs, createAccounts, login, sql, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
const phone = `6145${String(Date.now()).slice(-7)}`
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('contact timeline', () => {
  test.beforeAll(async () => {
    let owner: Account
    ;({ owner, reception } = await createAccounts(baseURL(), run))
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', '/api/accounts', { name: `Timeline WA ${run}`, phone_id: `t${run}`, business_id: `u${run}`, access_token: 'e2e-not-a-real-token' })
    await api.call('POST', '/api/contacts', { phone_number: phone, profile_name: `Tia Timeline ${run}`, whatsapp_account: `Timeline WA ${run}` })
    await api.dispose()
    const id = sql(`SELECT id FROM contacts WHERE phone_number = '${phone}'`)
    const org = sql(`SELECT organization_id FROM contacts WHERE id = '${id}'`)
    sql(`INSERT INTO wl.consent_events (organization_id, contact_id, action, source, given_at, evidence)
      VALUES ('${org}', '${id}', 'opt_in', 'staff', now() - interval '1 hour', 'paper form')`)
  })

  test('reception finds a contact and sees their history, without the number in full', async ({ page }) => {
    await login(page, reception)
    await page.getByRole('menuitem', { name: 'Contact timeline', exact: true }).click()
    await expect(page.getByTestId('timeline-pick')).toBeVisible()
    await page.getByTestId('timeline-search').fill(`Tia Timeline ${run}`)
    await page.getByTestId(`timeline-result-Tia Timeline ${run}`).click()
    await expect(page.getByTestId('timeline-contact')).toHaveText(`Tia Timeline ${run}`)
    await expect(page.getByTestId('timeline-item-consent')).toContainText('Opted in')
    await expect(page.getByTestId('timeline-items')).not.toContainText('paper form')
    await expect(page.locator('body')).not.toContainText(phone)
  })

  test('an unknown contact says so', async ({ page }) => {
    await login(page, reception)
    await page.goto('/x/timeline?contact=00000000-0000-0000-0000-000000000000')
    await expect(page.getByTestId('timeline-notfound')).toBeVisible()
  })
})
