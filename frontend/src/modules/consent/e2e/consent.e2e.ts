import { randomUUID } from 'node:crypto'
import { expect, request, test } from '@playwright/test'
import { PASSWORD, apiAs, createAccounts, csrfFrom, login, sql, type Account } from '../../practice/e2e/setup'

// One journey per user story in wl-docs/modules/consent/requirements.md
// (test-plan.md, "End to end"). They run in order: each builds on the last.
//
// There's no WhatsApp in CI. A patient's STOP is delivered the way Weline
// delivers it: a message.incoming POST to the hooks receiver with the
// organization's token, from a managed webhook row this journey creates. The
// confirmation reply goes through Weline's real send path and fails at Meta
// (fake token), which still stores it in the chat.

const run = Date.now().toString(36)
const account = `Consent WA ${run}`
const base = String(Date.now()).slice(-7)
const phones = { pia: `6146${base}`, quinn: `6147${base}`, ray: `6148${base}`, sam: `6149${base}` }
const names = { pia: `Pia ${run}`, quinn: `Quinn ${run}`, ray: `Ray ${run}`, sam: `Sam ${run}` }
const tag = `newsletter-${run}`
const template = `consent_promo_${run}`
const hookToken = `e2e-${randomUUID()}`

let owner: Account
let reception: Account
let orgId = ''
const ids: Record<string, string> = {}

const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

/** Delivers a WhatsApp message from a contact, as Weline's webhook would. */
async function patientSays(contact: keyof typeof phones, text: string) {
  const ctx = await request.newContext({ baseURL: baseURL() })
  const res = await ctx.post('/x/hooks/weline', {
    headers: { 'X-Weline-Org': orgId, 'X-Weline-Hook-Token': hookToken },
    data: {
      event: 'message.incoming',
      timestamp: new Date().toISOString(),
      data: { message_id: randomUUID(), contact_id: ids[contact], message_type: 'text', content: text, direction: 'incoming', whatsapp_account: account }
    }
  })
  expect(res.ok(), `hook delivery: ${await res.text()}`).toBeTruthy()
  await ctx.dispose()
}

test.describe.serial('consent', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
    orgId = sql(`SELECT organization_id FROM users WHERE email = '${owner.email}'`)
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', '/api/accounts', { name: account, phone_id: `c${run}`, business_id: `d${run}`, access_token: 'e2e-not-a-real-token' })
    await api.call('POST', '/api/tags', { name: tag, color: 'blue' })
    for (const k of Object.keys(phones) as (keyof typeof phones)[]) {
      await api.call('POST', '/api/contacts', { phone_number: phones[k], profile_name: names[k], whatsapp_account: account, tags: [tag] })
      ids[k] = sql(`SELECT id FROM contacts WHERE phone_number = '${phones[k]}'`)
    }
    await api.call('POST', '/api/templates', {
      whatsapp_account: account, name: template, language: 'en', category: 'MARKETING', header_type: 'NONE', body_content: 'News from the clinic.'
    })
    await api.dispose()
    sql(`UPDATE templates SET status = 'APPROVED' WHERE name = '${template}'`)
    // The managed webhook row the hooks receiver checks (normally made by
    // WELINE_PUBLIC_URL setup); the URL is never reachable from CI.
    sql(`UPDATE webhooks SET is_active = false WHERE organization_id = '${orgId}' AND name = 'Weline modules (managed, do not edit)'`)
    sql(`INSERT INTO webhooks (id, organization_id, name, url, events, headers, secret, is_active, created_at, updated_at)
      VALUES (gen_random_uuid(), '${orgId}', 'Weline modules (managed, do not edit)', 'https://hooks.invalid/x/hooks/weline',
        '["message.incoming"]', '{"X-Weline-Org": "${orgId}", "X-Weline-Hook-Token": "${hookToken}"}', '', true, now(), now())`)
  })

  test.afterAll(() => {
    sql(`DELETE FROM webhooks WHERE organization_id = '${orgId}' AND url = 'https://hooks.invalid/x/hooks/weline'`)
  })

  test('patient replies STOP, gets a confirmation and is opted out; START opts back in', async ({ page }) => {
    await patientSays('pia', 'Stop!')
    expect(sql(`SELECT marketing_opt_out FROM contacts WHERE id = '${ids.pia}'`)).toBe('t')
    expect(sql(`SELECT tags ? 'opted-out' FROM contacts WHERE id = '${ids.pia}'`)).toBe('t')
    expect(sql(`SELECT count(*) FROM messages WHERE contact_id = '${ids.pia}' AND direction = 'outgoing' AND content LIKE '%Reply START%'`)).toBe('1')

    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Consent', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/consent$/)
    const row = page.getByTestId(`consent-row-${names.pia}`)
    await expect(row).toContainText('Opted out')
    await expect(row).toContainText('WhatsApp keyword')
    await expect(row).toContainText('Replied “STOP”')
    await expect(row).not.toContainText(phones.pia)

    await patientSays('pia', 'STOP') // again: nothing new
    await patientSays('pia', 'start')
    expect(sql(`SELECT marketing_opt_out FROM contacts WHERE id = '${ids.pia}'`)).toBe('f')
    expect(sql(`SELECT count(*) FROM wl.consent_events WHERE contact_id = '${ids.pia}'`)).toBe('2')
    await page.reload()
    await expect(page.getByTestId(`consent-row-${names.pia}`).first()).toContainText('Opted in')
  })

  test('manager records an opt-in by hand and imports a CSV of opt-ins', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/consent')
    await page.getByTestId('consent-find').click()
    await page.getByTestId('consent-search').fill(names.quinn)
    await page.getByTestId(`consent-result-${names.quinn}`).click()
    await page.getByTestId('consent-record-in').click()
    await page.getByTestId('consent-record-save').click()
    await expect(page.getByTestId('consent-save-error')).toContainText('Say how they agreed')
    await page.getByTestId('consent-evidence').fill('Signed new patient form')
    await page.getByTestId('consent-record-save').click()
    await expect(page.getByTestId('consent-contact').getByTestId('state-opted_in')).toBeVisible()
    await expect(page.getByTestId('consent-contact-history')).toContainText('Signed new patient form')
    await page.keyboard.press('Escape')

    await page.getByTestId('consent-import').click()
    const csv = `phone,given_at,note\n0${phones.ray.slice(2)},2026-02-01,web form\n${phones.sam},2026-02-02,\n61400000999,2026-02-03,\n`
    await page.getByTestId('consent-import-file').setInputFiles({ name: 'optins.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) })
    await page.getByTestId('consent-import-check').click()
    await expect(page.getByTestId('consent-import-will')).toHaveText('2 will be recorded as opted in')
    await expect(page.getByTestId('consent-import-unknown')).toContainText('1 numbers')
    await page.getByTestId('consent-import-evidence').fill(`Newsletter sign-ups ${run}`)
    await page.getByTestId('consent-import-confirm').click()
    await expect(page.getByTestId(`consent-row-${names.ray}`)).toContainText('Import')
    await expect(page.getByTestId(`consent-row-${names.ray}`)).toContainText('web form')
    expect(sql(`SELECT count(*) FROM contacts WHERE phone_number = '61400000999'`)).toBe('0')
  })

  test('with "marketing only to people who opted in", Audiences leaves out the rest', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/consent/settings')
    await page.getByTestId('consent-require-opt-in').click()
    await page.getByTestId('consent-settings-save').click()
    await expect(page.getByText('Saved')).toBeVisible()
    await page.goto('/x/consent')
    await expect(page.getByTestId('consent-require')).toHaveText('Marketing goes only to people who opted in.')

    const api = await apiAs(baseURL(), owner)
    const templateId = sql(`SELECT id FROM templates WHERE name = '${template}'`)
    const rules = { match: 'all', conditions: [{ field: 'tag', op: 'has', value: tag }] }
    const p = await api.call('POST', '/api/x/audiences/preview', { rules, template_id: templateId })
    // Pia, Quinn, Ray and Sam all opted in now.
    expect(p.recipients).toBe(4)
    await api.call('POST', '/api/contacts', { phone_number: `6145${base}`, profile_name: `Una ${run}`, whatsapp_account: account, tags: [tag] })
    const p2 = await api.call('POST', '/api/x/audiences/preview', { rules, template_id: templateId })
    expect(p2.recipients).toBe(4)
    expect(p2.excluded.no_opt_in).toBe(1) // Una: no recorded opt-in
    const consentRule = await api.call('POST', '/api/x/audiences/preview', {
      rules: { match: 'all', conditions: [{ field: 'tag', op: 'has', value: tag }, { field: 'consent', op: 'not_recorded' }] }
    })
    expect(consentRule.matched).toBe(1)
    await api.call('PUT', '/api/x/consent/settings', { require_opt_in: false }) // leave the org as it was
    await api.dispose()
  })

  test('reception records an opt-out at the desk and cannot change settings', async ({ page }) => {
    await login(page, reception)
    await page.goto('/x/consent')
    await expect(page.getByTestId('consent-tab-settings')).toHaveCount(0)
    await page.getByTestId('consent-find').click()
    await page.getByTestId('consent-search').fill(names.sam)
    await page.getByTestId(`consent-result-${names.sam}`).click()
    await page.getByTestId('consent-record-out').click()
    await page.getByTestId('consent-evidence').fill('Asked at reception')
    await page.getByTestId('consent-record-save').click()
    await expect(page.getByTestId('consent-contact').getByTestId('state-opted_out')).toBeVisible()
    await page.keyboard.press('Escape')
    const row = page.getByTestId(`consent-row-${names.sam}`).first()
    await expect(row).toContainText('Recorded by staff')
    await expect(row).toContainText(reception.full_name)
    expect(sql(`SELECT marketing_opt_out FROM contacts WHERE id = '${ids.sam}'`)).toBe('t')

    await page.goto('/x/consent/settings')
    await expect(page).not.toHaveURL(/\/x\/consent\/settings$/)
    const ctx = await request.newContext({ baseURL: baseURL() })
    const signIn = await ctx.post('/api/auth/login', { data: { email: reception.email, password: PASSWORD } })
    const refused = await ctx.put('/api/x/consent/settings', { headers: { 'X-CSRF-Token': csrfFrom(signIn.headersArray()) }, data: { require_opt_in: true } })
    expect(refused.status()).toBe(403)
    await ctx.dispose()
  })

  test('history filters by source', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/consent')
    await page.getByTestId('consent-filter-source').click()
    await page.getByRole('option', { name: 'Import', exact: true }).click()
    await expect(page.getByTestId(`consent-row-${names.ray}`)).toBeVisible()
    await expect(page.getByTestId(`consent-row-${names.pia}`)).toHaveCount(0)
    await page.getByTestId('consent-filter-source').click()
    await page.getByRole('option', { name: 'WhatsApp keyword', exact: true }).click()
    await expect(page.getByTestId(`consent-row-${names.pia}`).first()).toBeVisible()
    await expect(page.getByTestId(`consent-row-${names.ray}`)).toHaveCount(0)
  })
})
