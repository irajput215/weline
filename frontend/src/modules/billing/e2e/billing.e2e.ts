import { expect, test } from '@playwright/test'
import { createAccounts, login, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('billing', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('an admin sees the trial, the plans and usage; nothing can be bought without Stripe keys', async ({ page }) => {
    test.skip(process.env.WELINE_BILLING !== 'on', 'needs WELINE_BILLING=on on the server; set in CI')
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Plan & billing', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/billing$/)
    await expect(page.getByTestId('billing-plan')).toHaveText('Trial')
    await expect(page.getByTestId('billing-trial-ends')).toBeVisible()
    await expect(page.getByTestId('billing-plan-clinic')).toContainText('10,000')
    await expect(page.getByTestId('billing-stripe-not-ready')).toBeVisible()
    await expect(page.getByTestId('billing-choose-starter')).toHaveCount(0)
    await expect(page.getByTestId('billing-usage-broadcast_recipients')).toContainText('/ 200')
    await expect(page.getByTestId('billing-usage-contacts')).toContainText('Shown, not blocked')
    await expect(page.getByTestId('billing-invoices-empty')).toBeVisible()
    await expect(page.getByTestId('billing-meta')).toContainText('bills you directly')
  })

  test('with billing off, usage shows and nothing is limited', async ({ page }) => {
    test.skip(process.env.WELINE_BILLING === 'on', 'runs when billing is off')
    await login(page, owner)
    await page.goto('/x/billing')
    await expect(page.getByTestId('billing-disabled')).toBeVisible()
    await expect(page.getByTestId('billing-usage-contacts')).toContainText('no limit')
  })

  test('reception has no billing page', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Plan & billing', exact: true })).toHaveCount(0)
  })
})
