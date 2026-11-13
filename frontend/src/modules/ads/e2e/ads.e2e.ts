import { randomUUID } from 'node:crypto'
import { expect, request, test } from '@playwright/test'
import { apiAs, createAccounts, login, sql, type Account } from '../../practice/e2e/setup'

// One journey per user story in wl-docs/modules/ads/requirements.md
// (test-plan.md, "End to end"). They run in order: each builds on the last.
//
// There's no WhatsApp in CI. A patient's first message is delivered the way
// Weline delivers it: a message.incoming POST to the hooks receiver with the
// organization's token, from a managed webhook row this journey creates.

const run = Date.now().toString(36)
const account = `Ads WA ${run}`
const digits = String(Date.now()).slice(-6)
const code = `FL${digits}`
const name = `Flu shot ad ${run}`
const campaign = `Flu season ${run}`
const phones = { uma: `6145${digits}1`, vic: `6145${digits}2` }
const names = { uma: `Uma ${run}`, vic: `Vic ${run}` }
const hookToken = `e2e-${randomUUID()}`

let owner: Account
let reception: Account
let orgId = ''
let entryId = ''
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

test.describe.serial('ads', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
    orgId = sql(`SELECT organization_id FROM users WHERE email = '${owner.email}'`)
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', '/api/accounts', { name: account, phone_id: `a${run}`, business_id: `b${run}`, access_token: 'e2e-not-a-real-token' })
    for (const k of Object.keys(phones) as (keyof typeof phones)[]) {
      await api.call('POST', '/api/contacts', { phone_number: phones[k], profile_name: names[k], whatsapp_account: account })
      ids[k] = sql(`SELECT id FROM contacts WHERE phone_number = '${phones[k]}'`)
    }
    await api.dispose()
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

  test('manager creates an entry point with a campaign and gets its link and message', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Entry points', exact: true }).click()
    await expect(page).toHaveURL(/\/x\/ads$/)
    await expect(page.getByTestId('ads-empty')).toBeVisible()

    await page.getByTestId('ads-new').click()
    await page.getByTestId('ads-name').fill(name)
    await expect(page.getByTestId('ads-code')).not.toHaveValue('', { timeout: 2000 }) // suggested from the name
    await page.getByTestId('ads-code').fill(code)
    await expect(page.getByTestId('ads-text')).toHaveValue(`Hi! I'd like to know more [${code}]`)
    await page.getByTestId('ads-campaign').fill(campaign)
    await page.getByTestId('ads-number').fill('+61 400 000 000')
    await expect(page.getByTestId('ads-link-preview')).toContainText('https://wa.me/61400000000?text=')
    await page.getByTestId('ads-save').click()

    await expect(page).toHaveURL(/\/x\/ads\/[0-9a-f-]{36}$/)
    entryId = page.url().split('/').pop() ?? ''
    await expect(page.getByTestId('ads-detail-code')).toHaveText(code)
    await expect(page.getByTestId('ads-detail-link')).toContainText(`%5B${code}%5D`)
    await expect(page.getByTestId('ads-detail-text')).toContainText(`[${code}]`)
    await expect(page.getByTestId('ads-detail-tag')).toHaveText(`from-${code.toLowerCase()}`)
    await expect(page.getByTestId('ads-arrivals-empty')).toBeVisible()

    // The same code again is refused on the code field.
    await page.goto('/x/ads')
    await page.getByTestId('ads-new').click()
    await page.getByTestId('ads-name').fill('Another ad')
    await page.getByTestId('ads-code').fill(code)
    await page.getByTestId('ads-number').fill('61400000000')
    await page.getByTestId('ads-save').click()
    await expect(page.getByTestId('ads-code-error')).toContainText('already used')
  })

  test('a new contact messages with the code: one chat, one new contact, tagged', async ({ page }) => {
    await patientSays('uma', `Hi! I'd like to know more [${code.toLowerCase()}]`)
    await patientSays('uma', `${code} again`) // same day: still one arrival
    expect(sql(`SELECT count(*) FROM wl.ads_arrivals WHERE contact_id = '${ids.uma}'`)).toBe('1')
    expect(sql(`SELECT tags ? 'from-${code.toLowerCase()}' FROM contacts WHERE id = '${ids.uma}'`)).toBe('t')
    await patientSays('vic', 'Just a question, no code')
    expect(sql(`SELECT count(*) FROM wl.ads_arrivals WHERE contact_id = '${ids.vic}'`)).toBe('0')

    await login(page, owner)
    await page.goto(`/x/ads/${entryId}`)
    await expect(page.getByTestId('ads-detail-conversations')).toHaveText('1')
    await expect(page.getByTestId('ads-detail-new_contacts')).toHaveText('1')
    const row = page.getByTestId(`ads-arrival-${names.uma}`)
    await expect(row).toContainText('New')
    await expect(row).not.toContainText(phones.uma)
    await expect(page.getByTestId('ads-delete')).toHaveCount(0) // has arrivals: archive only
  })

  test('the contact opts in and books: the entry point counts both', async ({ page }) => {
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', `/api/x/consent/contacts/${ids.uma}`, { action: 'opt_in', evidence: 'said yes at the desk' })
    await api.dispose()
    // A booking made in Weline's built-in Booking (practice rows made directly: setting up Practice is its own journey).
    sql(`WITH l AS (INSERT INTO wl.practice_locations (organization_id, name) VALUES ('${orgId}', 'Ads loc ${run}') RETURNING id),
      p AS (INSERT INTO wl.practice_practitioners (organization_id, display_name, role) VALUES ('${orgId}', 'Dr Ads', 'GP') RETURNING id),
      s AS (INSERT INTO wl.practice_services (organization_id, name, duration_minutes, allowed_roles) VALUES ('${orgId}', 'Ads svc ${run}', 15, '["GP"]') RETURNING id)
      INSERT INTO wl.booking_appointments (organization_id, reference, contact_id, practitioner_id, service_id, location_id, starts_at, ends_at, source)
      SELECT '${orgId}', upper(substr(md5(random()::text), 1, 8)), '${ids.uma}', p.id, s.id, l.id, now() + interval '3 days', now() + interval '3 days 15 minutes', 'whatsapp'
      FROM l, p, s`)

    await login(page, owner)
    await page.goto('/x/ads')
    await expect(page.getByTestId(`ads-${code}-conversations`)).toHaveText('1')
    await expect(page.getByTestId(`ads-${code}-opted_in`)).toHaveText('1')
    await expect(page.getByTestId(`ads-${code}-booked`)).toHaveText('1')
    const totals = page.getByTestId('ads-campaigns')
    await expect(totals).toContainText(campaign)
    await expect(totals.getByTestId('ads-campaign-booked')).toHaveText('1')
  })

  test('reception has no Entry points pages but sees the tag on the contact', async ({ page }) => {
    await login(page, reception)
    await expect(page.getByRole('menuitem', { name: 'Entry points', exact: true })).toHaveCount(0)
    await page.goto('/x/ads')
    await expect(page).not.toHaveURL(/\/x\/ads/)
    const api = await apiAs(baseURL(), reception)
    const contact = await api.call('GET', `/api/contacts/${ids.uma}`)
    await api.dispose()
    expect(JSON.stringify(contact)).toContain(`from-${code.toLowerCase()}`)
  })

  test('manager archives the entry point; the code stops counting', async ({ page }) => {
    await login(page, owner)
    await page.goto(`/x/ads/${entryId}`)
    await page.getByTestId('ads-archive').click()
    await page.getByRole('button', { name: 'Archive', exact: true }).last().click()
    await expect(page.getByTestId('ads-detail-archived')).toBeVisible()
    await expect(page.getByTestId('ads-edit')).toHaveCount(0)

    await patientSays('vic', `Hi [${code}]`)
    expect(sql(`SELECT count(*) FROM wl.ads_arrivals WHERE contact_id = '${ids.vic}'`)).toBe('0')
    await page.goto('/x/ads')
    await expect(page.getByTestId(`ads-row-${code}`)).toHaveCount(0)
    await page.getByTestId('ads-show-archived').click()
    await expect(page.getByTestId(`ads-row-${code}`)).toContainText('Archived')
  })
})
