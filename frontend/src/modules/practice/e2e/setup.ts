import { execFileSync } from 'node:child_process'
import { expect, request, type APIRequestContext, type Page } from '@playwright/test'

// Test accounts for the practice journeys. They live in the default organization,
// whose system roles got the wl_practice_* grants when the module migration ran.
// Names carry a per-run suffix so the journeys can run again on the same database.

const ADMIN_EMAIL = process.env.WL_E2E_ADMIN_EMAIL || 'admin@admin.com'
const ADMIN_PASSWORD = process.env.WL_E2E_ADMIN_PASSWORD || 'admin'
export const PASSWORD = 'Practice-e2e-2026!'

export interface Account {
  email: string
  full_name: string
}

export function csrfFrom(headers: Array<{ name: string; value: string }>): string {
  for (const h of headers) {
    const m = h.name.toLowerCase() === 'set-cookie' && h.value.match(/whm_csrf=([^;]+)/)
    if (m) return m[1]
  }
  return ''
}

/** Creates an owner (admin role) and a receptionist (agent role) for this run. */
export async function createAccounts(baseURL: string, run: string): Promise<{ owner: Account; reception: Account }> {
  const ctx = await request.newContext({ baseURL })
  const login = await ctx.post('/api/auth/login', { data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD } })
  expect(login.ok(), 'superadmin login (set WL_E2E_ADMIN_EMAIL / WL_E2E_ADMIN_PASSWORD)').toBeTruthy()
  const headers = { 'X-CSRF-Token': csrfFrom(login.headersArray()) }

  const roles = (await (await ctx.get('/api/roles')).json()).data.roles as Array<{ id: string; name: string }>
  const roleId = (name: string) => roles.find(r => r.name === name)?.id

  const owner = { email: `owner-${run}@e2e.test`, full_name: `Olivia Owner ${run}` }
  const reception = { email: `reception-${run}@e2e.test`, full_name: `Rita Reception ${run}` }
  for (const [acct, role] of [[owner, 'admin'], [reception, 'agent']] as const) {
    const res = await ctx.post('/api/users', {
      headers,
      data: { ...acct, password: PASSWORD, role_id: roleId(role), is_active: true }
    })
    expect(res.ok(), `create ${role} user: ${await res.text()}`).toBeTruthy()
  }
  await ctx.dispose()
  return { owner, reception }
}

export async function login(page: Page, acct: Account) {
  await page.context().clearCookies()
  await page.goto('/login', { waitUntil: 'domcontentloaded' })
  await page.locator('input[type="email"]').fill(acct.email)
  await page.locator('input[type="password"]').fill(PASSWORD)
  await page.locator('button[type="submit"]').click()
  await page.waitForURL(url => !url.pathname.includes('/login'))
}

/** Picks an option in a shadcn Select opened from the given trigger. */
export async function choose(page: Page, triggerTestId: string, option: string | RegExp) {
  await page.getByTestId(triggerTestId).click()
  await page.getByRole('option', { name: option, exact: true }).click()
}

/** An API client logged in as acct, sending the CSRF header on writes. */
export async function apiAs(baseURL: string, acct: Account) {
  const ctx: APIRequestContext = await request.newContext({ baseURL })
  const res = await ctx.post('/api/auth/login', { data: { email: acct.email, password: PASSWORD } })
  expect(res.ok(), `login ${acct.email}`).toBeTruthy()
  const headers = { 'X-CSRF-Token': csrfFrom(res.headersArray()) }
  const call = async (method: 'GET' | 'POST' | 'PUT', path: string, data?: unknown) => {
    const r = await ctx.fetch(path, { method, headers, data })
    const body = await r.json()
    expect(r.ok(), `${method} ${path}: ${body.message ?? ''}`).toBeTruthy()
    return body.data
  }
  return { call, dispose: () => ctx.dispose() }
}

/**
 * Runs SQL on the e2e database (the WHATOMATE_DATABASE__* settings the e2e
 * script exports). Only for what CI can't do through the app: Meta approving a
 * template, WhatsApp delivery results, the managed webhook row.
 */
export function sql(query: string): string {
  const env = process.env
  return execFileSync(
    'psql',
    ['-h', env.WHATOMATE_DATABASE__HOST ?? 'localhost', '-p', env.WHATOMATE_DATABASE__PORT ?? '5432',
      '-U', env.WHATOMATE_DATABASE__USER ?? 'postgres', '-d', env.WHATOMATE_DATABASE__NAME ?? 'weline_e2e',
      '-v', 'ON_ERROR_STOP=1', '-Atc', query],
    { env: { ...env, PGPASSWORD: env.WHATOMATE_DATABASE__PASSWORD ?? 'postgres' }, encoding: 'utf8' }
  ).trim()
}
