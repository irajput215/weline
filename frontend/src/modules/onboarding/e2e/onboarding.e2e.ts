import { expect, test } from '@playwright/test'
import { PASSWORD, choose, createAccounts, login, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('onboarding', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('a new business signs up, signs in and works through the checklist', async ({ page }) => {
    test.skip(!process.env.WL_E2E_CAPTCHA, 'needs Turnstile test keys and internet (set WL_E2E_CAPTCHA=1); runs in CI')
    test.skip(process.env.WELINE_SIGNUP !== 'open', 'needs WELINE_SIGNUP=open on the server; set in CI')
    const email = `signup-${run}@e2e.test`
    await page.context().clearCookies()
    await page.goto('/x/onboarding/signup')
    await page.getByTestId('signup-business').fill(`Signup Physio ${run}`)
    await page.getByTestId('signup-name').fill(`Sam Signup ${run}`)
    await page.getByTestId('signup-email').fill(email)
    await page.getByTestId('signup-password').fill(PASSWORD)
    await page.getByTestId('signup-tz').selectOption('Australia/Perth')
    await page.getByTestId('signup-terms').check()
    await page.waitForFunction(() => {
      const el = document.querySelector('[name="cf-turnstile-response"]') as HTMLInputElement | null
      return !!el && el.value.length > 0
    }, undefined, { timeout: 30_000 })
    await page.getByTestId('signup-submit').click()
    await expect(page.getByTestId('signup-done')).toBeVisible()

    await login(page, { email, full_name: `Sam Signup ${run}` })
    await page.goto('/x/onboarding')
    await expect(page.getByTestId('onboarding-progress')).toHaveText('1 of 6 done')
    await expect(page.getByTestId('onboarding-step-business')).toHaveAttribute('data-done', 'true')
    await expect(page.getByTestId('onboarding-step-whatsapp')).toHaveAttribute('data-done', 'false')
    await choose(page, 'onboarding-vertical', 'Allied health (physio, chiro, psychology…)')
    await expect(page.getByTestId('onboarding-progress')).toHaveText('2 of 6 done')

    // The new organization's admin can open the module pages (permissions granted at sign-up).
    await page.getByTestId('onboarding-go-system').click()
    await expect(page).toHaveURL(/\/x\/connectors$/)
  })

  test('an existing admin sees the checklist and can hide it', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Setup checklist', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/onboarding$/)
    await expect(page.getByTestId('onboarding-step-team')).toBeVisible()
    await page.getByTestId('onboarding-dismiss').click()
    await expect(page.getByTestId('onboarding-hidden')).toBeVisible()
    await page.getByTestId('onboarding-dismiss').click()
    await expect(page.getByTestId('onboarding-hidden')).toHaveCount(0)
  })

  test('reception has no setup checklist', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Setup checklist', exact: true })).toHaveCount(0)
    await page.goto('/x/onboarding')
    await expect(page).not.toHaveURL(/\/x\/onboarding$/)
  })

  test('the source of the running version is linked', async ({ request }) => {
    const res = await request.get('/x/source', { maxRedirects: 0 })
    expect(res.status()).toBe(302)
    expect(res.headers().location).toContain('github.com')
  })
})
