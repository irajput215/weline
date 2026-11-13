import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const a = vi.hoisted(() => ({ rules: vi.fn(), createRule: vi.fn(), updateRule: vi.fn(), deleteRule: vi.fn(), sends: vi.fn(), due: vi.fn(), sendRecalls: vi.fn(), templates: vi.fn() }))
vi.mock('../api', () => ({ remindersApi: a }))

import { useRemindersStore } from './reminders'

const rule = { kind: 'reminder' as const, name: 'Day before', offset_minutes: 1440, recall_days: 0, template_name: 't', language: 'en', variables: [], service_ids: [], enabled: true }

describe('reminders store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.values(a).forEach(fn => fn.mockReset())
  })

  it('loads each part with its own state; 403 is forbidden', async () => {
    const store = useRemindersStore()
    a.rules.mockResolvedValue({ rules: null })
    await store.loadRules()
    expect(store.state.rules).toBe('ready')
    expect(store.rules).toEqual([])
    a.due.mockRejectedValueOnce({ response: { status: 403 } })
    await store.loadDue('r1')
    expect(store.state.due).toBe('forbidden')
    a.sends.mockRejectedValueOnce(new Error('offline'))
    await store.loadSends()
    expect(store.state.sends).toBe('error')
    a.templates.mockRejectedValueOnce({ response: { status: 403 } })
    await store.loadTemplates()
    expect(store.templates).toEqual([])
  })

  it('creates, updates and deletes rules, keeping the server message on failure', async () => {
    const store = useRemindersStore()
    a.createRule.mockResolvedValueOnce({ rule: { ...rule, id: 'r1' } })
    expect(await store.saveRule(rule)).toBe(true)
    a.updateRule.mockResolvedValueOnce({ rule: { ...rule, id: 'r1', name: 'Two days' } })
    expect(await store.saveRule({ ...rule, id: 'r1', name: 'Two days' })).toBe(true)
    expect(a.updateRule).toHaveBeenCalledWith('r1', expect.objectContaining({ name: 'Two days' }))
    expect(store.rules.map(r => r.name)).toEqual(['Two days'])
    a.createRule.mockRejectedValueOnce({ response: { status: 400, data: { message: 'template is not approved' } } })
    expect(await store.saveRule(rule)).toBe(false)
    expect(store.saveError).toBe('template is not approved')
    a.deleteRule.mockResolvedValueOnce({ deleted: true })
    await store.removeRule('r1')
    expect(store.rules).toEqual([])
  })

  it('sends recalls and drops them from the due list', async () => {
    const store = useRemindersStore()
    a.due.mockResolvedValue({ due: [{ customer_id: 'c1' }, { customer_id: 'c2' }] })
    await store.loadDue('r2')
    a.sendRecalls.mockResolvedValueOnce({ counts: { sent: 1, skipped: 0, not_due: 0, already: 0 } })
    expect((await store.sendRecalls('r2', ['c1']))?.sent).toBe(1)
    expect(store.due.map(d => d.customer_id)).toEqual(['c2'])
  })
})
