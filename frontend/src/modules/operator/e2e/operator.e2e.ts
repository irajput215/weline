import { expect, test } from '@playwright/test'
import { createAccounts, login, sql, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
let owner: Account
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'
const ADMIN_EMAIL = process.env.WL_E2E_ADMIN_EMAIL || 'admin@admin.com'
const ADMIN_PASSWORD = process.env.WL_E2E_ADMIN_PASSWORD || 'admin'

test.describe.serial('operator console', () => {
  test.beforeAll(async () => {
    ;({ owner } = await createAccounts(baseURL(), run))
  })

  test('a super admin sees every business, opens one, and the view is audited there', async ({ page }) => {
    await page.context().clearCookies()
    await page.goto('/login', { waitUntil: 'domcontentloaded' })
    await page.locator('input[type="email"]').fill(ADMIN_EMAIL)
    await page.locator('input[type="password"]').fill(ADMIN_PASSWORD)
    await page.locator('button[type="submit"]').click()
    await page.waitForURL(url => !url.pathname.includes('/login'))
    await page.getByRole('menuitem', { name: 'Operator console', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/operator$/)
    await expect(page.getByTestId('operator-stat-organizations')).toBeVisible()

    const org = sql(`SELECT o.name FROM organizations o JOIN users u ON u.organization_id = o.id WHERE u.email = '${owner.email}'`)
    const orgId = sql(`SELECT organization_id FROM users WHERE email = '${owner.email}'`)
    // Viewing is recorded in the operator's organization and, when it's another one, in the business's own log.
    const audited = () => Number(sql(`SELECT count(*) FROM wl.audit_events WHERE module = 'operator' AND entity_id = '${orgId}'`))
    const before = audited()
    await page.getByTestId('operator-search').fill(org)
    await page.getByTestId(`operator-tenant-${org}`).click()
    await expect(page.getByTestId('operator-health')).toBeVisible()
    await expect(page.getByTestId('operator-setup')).toContainText('of 6 steps')
    await expect.poll(audited).toBeGreaterThan(before)
  })

  test("a business's own admin has no console", async ({ page }) => {
    await login(page, owner)
    await expect(page.getByRole('menuitem', { name: 'Operator console', exact: true })).toHaveCount(0)
    const res = await page.request.get('/api/x/operator/overview')
    expect(res.status()).toBe(403)
  })
})
