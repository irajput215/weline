import { expect, test } from '@playwright/test'
import { createAccounts, login, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
let reception: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('ai agent', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
  })

  test('a manager sees the provider warning, adds knowledge and turns the agent on', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'AI agent', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/ai$/)
    await expect(page.getByTestId('ai-no-provider')).toBeVisible()

    await page.getByTestId('ai-tab-knowledge').click()
    await expect(page.getByTestId('ai-knowledge-empty')).toBeVisible()
    await page.getByTestId('ai-faq-question').fill(`Is there parking ${run}?`)
    await page.getByTestId('ai-faq-answer').fill('Free parking behind the building on Bay Lane.')
    await page.getByTestId('ai-faq-add').click()
    await expect(page.getByTestId(`ai-knowledge-Is there parking ${run}?`)).toBeVisible()

    await page.getByTestId('ai-tab-settings').click()
    await page.getByTestId('ai-enabled').click()
    await page.getByTestId('ai-save').click()
    await expect(page.getByText('Saved').first()).toBeVisible()
    await page.reload()
    await page.getByTestId('ai-tab-settings').click()
    await expect(page.getByTestId('ai-enabled')).toHaveAttribute('aria-checked', 'true')

    await page.getByTestId('ai-tab-runs').click()
    await expect(page.getByTestId('ai-runs-empty')).toBeVisible()
  })

  test('reception sees the activity only', async ({ page }) => {
    await login(page, reception)
    await page.getByRole('menuitem', { name: 'AI agent', exact: true }).click()
    await expect(page.getByTestId('ai-tab-runs')).toBeVisible()
    await expect(page.getByTestId('ai-tab-settings')).toHaveCount(0)
    await expect(page.getByTestId('ai-tab-knowledge')).toHaveCount(0)
  })
})
