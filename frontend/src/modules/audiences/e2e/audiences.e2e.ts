import { execFileSync } from 'node:child_process'
import { expect, request, test } from '@playwright/test'
import { PASSWORD, apiAs, choose, createAccounts, csrfFrom, login, type Account } from '../../practice/e2e/setup'

// One journey per user story in wl-docs/modules/audiences/requirements.md
// (test-plan.md, "End to end"). They run in order: each builds on the last.
//
// There's no Meta or WhatsApp in CI, so two things are set straight in the
// database (the WHATOMATE_DATABASE__* settings the e2e script exports):
// Meta approving the template, and the delivery results of an earlier campaign.
// Contacts' "last wrote to us" and opt-out flags are set the same way, since
// only incoming WhatsApp messages and the opt-out keyword change them.

const run = Date.now().toString(36)
const account = `Aud WA ${run}`
const tag = `physio-${run}`
const template = `aud_flu_${run}`
const segment = `Physio recent ${run}`
const winter = `Winter ${run}`
const base = String(Date.now()).slice(-7)
const phones = {
  alice: `6141${base}`,
  ben: `6142${base}`,
  olive: `6143${base}`, // physio, but hasn't written in a year
  otto: `6144${base}`, // physio, recent, opted out of marketing
  mia: `6145${base}` // massage
}

let owner: Account
let reception: Account

function sql(query: string): string {
  const env = process.env
  return execFileSync(
    'psql',
    ['-h', env.WHATOMATE_DATABASE__HOST ?? 'localhost', '-p', env.WHATOMATE_DATABASE__PORT ?? '5432',
      '-U', env.WHATOMATE_DATABASE__USER ?? 'postgres', '-d', env.WHATOMATE_DATABASE__NAME ?? 'weline_e2e',
      '-v', 'ON_ERROR_STOP=1', '-Atc', query],
    { env: { ...env, PGPASSWORD: env.WHATOMATE_DATABASE__PASSWORD ?? 'postgres' }, encoding: 'utf8' }
  ).trim()
}

const baseURL = () => test.info().project.use.baseURL ?? 'http://localhost:8080'

async function recipientsOf(campaignId: string): Promise<Array<{ phone_number: string; recipient_name: string; template_params: Record<string, string> }>> {
  const api = await apiAs(baseURL(), owner)
  const data = await api.call('GET', `/api/campaigns/${campaignId}/recipients`)
  await api.dispose()
  return data.recipients
}

test.describe.serial('audiences', () => {
  test.beforeAll(async () => {
    ;({ owner, reception } = await createAccounts(baseURL(), run))
    const api = await apiAs(baseURL(), owner)
    await api.call('POST', '/api/accounts', { name: account, phone_id: `a${run}`, business_id: `b${run}`, access_token: 'e2e-not-a-real-token' })
    await api.call('POST', '/api/tags', { name: tag, color: 'green' })
    const contact = (name: string, phone: string, tags: string[]) =>
      api.call('POST', '/api/contacts', { phone_number: phone, profile_name: name, whatsapp_account: account, tags })
    await contact(`Alice ${run}`, phones.alice, [tag])
    await contact(`Ben ${run}`, phones.ben, [tag])
    await contact(`Olive ${run}`, phones.olive, [tag])
    await contact(`Otto ${run}`, phones.otto, [tag])
    await contact(`Mia ${run}`, phones.mia, ['massage'])
    await api.call('POST', '/api/templates', {
      whatsapp_account: account, name: template, language: 'en', category: 'MARKETING', header_type: 'NONE',
      body_content: 'Hi {{1}}, flu shots are in this week.'
    })
    await api.dispose()

    const all = Object.values(phones).map(p => `'${p}'`).join(',')
    const recent = [phones.alice, phones.ben, phones.otto, phones.mia].map(p => `'${p}'`).join(',')
    sql(`UPDATE templates SET status = 'APPROVED' WHERE name = '${template}'`)
    sql(`UPDATE contacts SET last_inbound_at = now() - interval '5 days' WHERE phone_number IN (${recent})`)
    sql(`UPDATE contacts SET last_inbound_at = now() - interval '400 days' WHERE phone_number = '${phones.olive}'`)
    sql(`UPDATE contacts SET marketing_opt_out = true WHERE phone_number = '${phones.otto}'`)
    // An earlier campaign: Alice read it, Ben and Mia only received it.
    sql(`WITH c AS (
        INSERT INTO bulk_message_campaigns (id, organization_id, whats_app_account, name, template_id, status, total_recipients,
          sent_count, delivered_count, read_count, created_by, created_at, updated_at, completed_at)
        SELECT gen_random_uuid(), t.organization_id, '${account}', '${winter}', t.id, 'completed', 3, 3, 3, 1,
          (SELECT id FROM users WHERE email = '${owner.email}'), now() - interval '3 days', now(), now() - interval '3 days'
        FROM templates t WHERE t.name = '${template}' RETURNING id)
      INSERT INTO bulk_message_recipients (id, campaign_id, phone_number, recipient_name, status, sent_at, delivered_at, read_at, created_at, updated_at)
      SELECT gen_random_uuid(), c.id, x.phone, '', x.status, now() - interval '3 days', now() - interval '3 days',
        CASE WHEN x.status = 'read' THEN now() - interval '3 days' END, now(), now()
      FROM c, (VALUES ('${phones.alice}', 'read'), ('${phones.ben}', 'delivered'), ('${phones.mia}', 'delivered')) AS x(phone, status)`)
    expect(sql(`SELECT count(*) FROM contacts WHERE phone_number IN (${all})`)).toBe('5')
  })

  test('manager builds a segment, sees the count and a masked sample, and saves it', async ({ page }) => {
    await login(page, owner)
    await page.getByRole('menuitem', { name: 'Segments' }).click()
    await expect(page).toHaveURL(/\/x\/audiences$/)
    await page.getByTestId('segment-new').click()

    await page.getByTestId('segment-name').fill(segment)
    await choose(page, 'condition-value', tag)
    await page.getByTestId('condition-add').click()
    const second = page.getByTestId('condition-1')
    await second.getByTestId('condition-field').click()
    await page.getByRole('option', { name: 'Last wrote to us', exact: true }).click()
    await expect(second.getByTestId('condition-value')).toHaveValue('90')

    // Alice, Ben and Otto (Olive wrote too long ago, Mia isn't tagged).
    await expect(page.getByTestId('preview-matched')).toHaveText('3 contacts match')
    const sample = page.getByTestId('preview-sample')
    await expect(sample).toContainText(`Alice ${run}`)
    await expect(sample).not.toContainText(phones.alice)
    await expect(sample).toContainText('•')

    await page.getByTestId('segment-save').click()
    await expect(page).toHaveURL(/\/x\/audiences\/segments\/[0-9a-f-]{36}$/)
    await page.getByTestId('audiences-tab-segments').click()
    await expect(page.getByTestId(`segment-row-${segment}`).getByTestId('segment-count')).toHaveText('3')
  })

  test('manager creates a draft campaign from it; the opted-out contact is left out', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/audiences')
    await page.getByRole('link', { name: segment }).click()
    await page.getByTestId('segment-create-campaign').click()

    const dialog = page.getByRole('dialog', { name: 'Create campaign' })
    await dialog.getByTestId('send-name').fill(`Flu invite ${run}`)
    await choose(page, 'send-account', account)
    await choose(page, 'send-template', `${template} (en)`)
    // {{1}} defaults to the contact's name.
    await expect(dialog.getByTestId('send-message-preview')).toContainText('flu shots are in this week')
    await expect(dialog.getByTestId('preview-recipients')).toHaveText('2 would get the message')
    await expect(dialog.getByTestId('excluded-opted_out')).toHaveText('1 opted out of marketing')

    await dialog.getByTestId('send-confirm').click()
    await page.getByRole('button', { name: 'Open campaign' }).click()
    await expect(page).toHaveURL(/\/campaigns\/[0-9a-f-]{36}$/)
    const campaignId = page.url().split('/').pop() as string

    const recips = await recipientsOf(campaignId)
    expect(recips.map(r => r.phone_number).sort()).toEqual([phones.alice, phones.ben])
    expect(recips.find(r => r.phone_number === phones.alice)?.template_params['1']).toBe(`Alice ${run}`)
    expect(sql(`SELECT status || ':' || total_recipients FROM bulk_message_campaigns WHERE id = '${campaignId}'`)).toBe('draft:2')
  })

  test('manager follows up an earlier campaign with only the people who did not read it', async ({ page }) => {
    await login(page, owner)
    await page.goto('/x/audiences/retarget')
    await page.getByTestId(`retarget-row-${winter}`).click()
    await expect(page.getByTestId('preview-recipients')).toHaveText('2 would get the message') // Ben and Mia
    await page.getByTestId('retarget-create-campaign').click()

    const dialog = page.getByRole('dialog', { name: 'Create campaign' })
    await expect(dialog.getByTestId('send-name')).toHaveValue(`${winter} – Received but didn't read`)
    await choose(page, 'send-account', account)
    await choose(page, 'send-template', `${template} (en)`)
    await expect(dialog.getByTestId('send-confirm')).toHaveText('Create draft for 2 people')
    await dialog.getByTestId('send-confirm').click()
    await page.getByRole('button', { name: 'Open campaign' }).click()
    await expect(page).toHaveURL(/\/campaigns\/[0-9a-f-]{36}$/)

    const recips = await recipientsOf(page.url().split('/').pop() as string)
    expect(recips.map(r => r.phone_number).sort()).toEqual([phones.ben, phones.mia])

    await page.goto('/x/audiences/history')
    await expect(page.getByTestId(`history-row-Flu invite ${run}`)).toContainText('Segment')
    await expect(page.getByTestId(`history-row-${winter} – Received but didn't read`)).toContainText("Retarget: Received but didn't read")
  })

  test('reception can view segments but not change them or create campaigns', async ({ page }) => {
    await login(page, reception)
    await page.goto('/x/audiences')
    await expect(page.getByTestId(`segment-row-${segment}`)).toBeVisible()
    await expect(page.getByTestId('segment-new')).toHaveCount(0)
    await page.getByRole('link', { name: segment }).click()
    await expect(page.getByTestId('builder-readonly')).toBeVisible()
    await expect(page.getByTestId('segment-save')).toHaveCount(0)
    await expect(page.getByTestId('segment-create-campaign')).toHaveCount(0)

    const ctx = await request.newContext({ baseURL: baseURL() })
    const signIn = await ctx.post('/api/auth/login', { data: { email: reception.email, password: PASSWORD } })
    const headers = { 'X-CSRF-Token': csrfFrom(signIn.headersArray()) }
    const refused = await ctx.post('/api/x/audiences/segments', { headers, data: { name: `Nope ${run}`, rules: { match: 'all', conditions: [] } } })
    expect(refused.status()).toBe(403)
    await ctx.dispose()
  })
})
