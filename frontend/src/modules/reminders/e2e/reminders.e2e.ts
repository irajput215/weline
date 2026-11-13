import { expect, test } from '@playwright/test'
import { apiAs, choose, createAccounts, login, sql, type Account } from '../../practice/e2e/setup'

const run = Date.now().toString(36)
const account = `Reminders WA ${run}`
const template = `appt_reminder_${run}`
let owner: Account
let reception: Account
let orgId = ''
const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

test.describe.serial('reminders', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
    orgId = sql(`SELECT organization_id FROM users WHERE email = '${owner.email}'`)
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', '/api/accounts', { name: account, phone_id: `r${run}`, business_id: `s${run}`, access_token: 'e2e-not-a-real-token' })
    await api.call('POST', '/api/templates', {
      whatsapp_account: account, name: template, language: 'en', category: 'UTILITY', header_type: 'NONE',
      body_content: 'Hi {{1}}, a reminder of your appointment on {{2}}.'
    })
    await api.dispose()
    // Meta approving the template, which CI can't do.
    sql(`UPDATE templates SET status = 'APPROVED' WHERE name = '${template}'`)
  })

  test('a manager adds a day-before reminder and sees what was sent', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Reminders & recalls', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/reminders$/)

    await page.getByTestId('reminders-add').click()
    await page.getByTestId('reminders-name').fill(`Day before ${run}`)
    await page.getByTestId('reminders-hours').fill('24')
    await choose(page, 'reminders-template', `${template} (en)`)
    await page.getByTestId('reminders-save').click()
    await expect(page.getByTestId(`reminders-rule-Day before ${run}`)).toContainText('24 h before')

    // A recall: switching the type drops variables a recall can't fill.
    await page.getByTestId('reminders-add').click()
    await choose(page, 'reminders-kind', 'Recall')
    await page.getByTestId('reminders-name').fill(`Six months ${run}`)
    await choose(page, 'reminders-template', `${template} (en)`)
    await page.getByTestId('reminders-save').click()
    await expect(page.getByTestId(`reminders-rule-Six months ${run}`)).toContainText('180 days after')

    // A send the sweep made (the sweep needs a live connector, so it's recorded directly).
    const ruleId = sql(`SELECT id FROM wl.reminder_rules WHERE organization_id = '${orgId}' AND name = 'Day before ${run}'`)
    sql(`INSERT INTO wl.reminder_sends (organization_id, rule_id, provider, target_id, start_at, status, reply)
      VALUES ('${orgId}', '${ruleId}', 'builtin', 'e2e-${run}', now() + interval '1 day', 'sent', 'confirmed')`)
    await page.getByTestId('reminders-tab-sends').click()
    await expect(page.getByTestId('reminders-send-sent').first()).toBeVisible()
    await expect(page.getByTestId('reminders-sends')).toContainText('Confirmed')

    await page.getByTestId('reminders-tab-recalls').click()
    await expect(page.getByTestId('reminders-due-empty')).toBeVisible()
  })

  test('reception sees rules and activity but cannot change them or send recalls', async ({ page }) => {
    await login(page, reception)
    await page.getByRole('menuitem', { name: 'Reminders & recalls', exact: true }).click()
    await expect(page.getByTestId(`reminders-rule-Day before ${run}`)).toBeVisible()
    await expect(page.getByTestId('reminders-add')).toHaveCount(0)
    await expect(page.getByTestId('reminders-tab-sends')).toBeVisible()
    await expect(page.getByTestId('reminders-tab-recalls')).toHaveCount(0)
  })
})
