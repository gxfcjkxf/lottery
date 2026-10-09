import { describe, expect, it, vi } from 'vitest'
import { PlatformApiError } from './platform-api'
import { createPlatformCommissionApi } from './commission-api'

const brand = '11111111-1111-4111-8111-111111111111'
const agentId = '22222222-2222-4222-8222-222222222222'
const memberId = '33333333-3333-4333-8333-333333333333'
const cycleId = '44444444-4444-4444-8444-444444444444'
const runId = '55555555-5555-4555-8555-555555555555'
const paymentId = '66666666-6666-4666-8666-666666666666'
const auditId = '77777777-7777-4777-8777-777777777777'
const at = '2026-10-07T12:00:00Z'
const huge = '9007199254740993'

const agent = {
  id: agentId, brand_id: brand, member_id: memberId, parent_id: null, depth: 1, path: [agentId], version: 2,
  config: { ratio: '0.1', mode: null, status: 'active', can_create_children: true }, effective_mode: 'loss',
  mode_source_agent_id: null, policy_version: 3, parent_version: null, created_by: memberId, created_at: at, updated_at: at,
}
const cycle = {
  id: cycleId, brand_id: brand, window_from: at, window_to: '2026-10-08T12:00:00Z', anchor_order_id: paymentId,
  calendar: { timezone: 'Asia/Singapore', cycle: 'weekly', boundary_time: '18:30:00', weekday: 1, month_day: null, short_month: '' },
  state: 'ready', version: 2, target_count: huge, scan_complete: true, current_run_id: runId, current_generation: huge,
  evidence_epoch: huge, evidence_current: true, calculated_count: huge, earning_count: huge, total_points: huge,
  created_by: memberId, creation_actor_type: 'admin', reason: 'cycle run', created_at: at, updated_at: at,
  last_error_code: null, creation_audit_log_id: auditId,
}
const earning = {
  id: agentId, brand_id: brand, cycle_id: cycleId, run_id: runId, agent_id: agentId, member_id: memberId,
  exact_amount: { numerator: huge, denominator: '3' }, points: huge, created_at: at,
}
const payment = {
  id: paymentId, brand_id: brand, cycle_id: cycleId, run_id: runId, state: 'paying', payout_mode: 'automatic', version: 1,
  evidence_epoch: huge, total_points: huge, paid_points: huge, target_count: huge, paid_count: huge,
  creation_audit_log_id: auditId, last_error_code: null, created_at: at, updated_at: at,
}
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { status: 200 })
const page = (items: unknown[], limit = 50, offset = 0) => ({ brand_id: brand, items, total_count: huge, limit, offset })

describe('platform commission read API', () => {
  it('accepts monthly windows and a genuine not-yet-calculated cycle without inventing a run', async () => {
    const monthly = { ...cycle, calendar: { timezone: 'Asia/Singapore', cycle: 'monthly', boundary_time: '18:30:00', weekday: null, month_day: 31, short_month: 'last_day' },
      state: 'enumerating', scan_complete: false, current_run_id: null, current_generation: null, evidence_epoch: null, evidence_current: false,
      calculated_count: '0', earning_count: '0', total_points: '0' };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(monthly));
    expect(await createPlatformCommissionApi(fetcher).cycle(brand, cycleId)).toEqual(monthly);
  })
  it('uses exact GET endpoints, brand scope, pagination and current DTOs', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ brand_id: brand, parent_id: null, items: [agent], limit: 50, offset: 0, total_count: huge }))
      .mockResolvedValueOnce(ok(agent))
      .mockResolvedValueOnce(ok(page([cycle])))
      .mockResolvedValueOnce(ok(cycle))
      .mockResolvedValueOnce(ok({ ...page([earning]), cycle_id: cycleId }))
      .mockResolvedValueOnce(ok(page([payment])))
      .mockResolvedValueOnce(ok(payment))
    const api = createPlatformCommissionApi(fetcher)

    await expect(api.tree(brand)).resolves.toMatchObject({ items: [{ id: agentId }], total_count: huge })
    await expect(api.agent(brand, agentId)).resolves.toMatchObject({ config: { ratio: '0.1' } })
    await expect(api.cycles(brand)).resolves.toMatchObject({ items: [{ calendar: { timezone: 'Asia/Singapore' } }] })
    await expect(api.cycle(brand, cycleId)).resolves.toMatchObject({ total_points: huge })
    await expect(api.earnings(brand, cycleId)).resolves.toMatchObject({ items: [{ exact_amount: { numerator: huge }, points: huge }] })
    await expect(api.payments(brand)).resolves.toMatchObject({ items: [{ total_points: huge }] })
    await expect(api.payment(brand, paymentId)).resolves.toMatchObject({ id: paymentId, paid_points: huge })

    expect(fetcher.mock.calls.map(call => call[0])).toEqual([
      '/api/v1/platform/agents/tree?limit=50&offset=0',
      `/api/v1/platform/agents/${agentId}`,
      '/api/v1/platform/commission-cycles?limit=50&offset=0',
      `/api/v1/platform/commission-cycles/${cycleId}`,
      `/api/v1/platform/commission-cycles/${cycleId}/earnings?limit=50&offset=0`,
      '/api/v1/platform/commission-payments?limit=50&offset=0',
      `/api/v1/platform/commission-payments/${paymentId}`,
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe('GET')
      expect(init?.credentials).toBe('same-origin')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
      expect(init?.body).toBeUndefined()
    }
    expect(Object.keys(api)).toEqual(['tree', 'agent', 'cycles', 'cycle', 'earnings', 'payments', 'payment'])
  })

  it('validates brands, IDs, page arguments and echoed pagination before accepting responses', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ brand_id: brand, parent_id: null, items: [], limit: 50, offset: 0, total_count: '0' }))
    const api = createPlatformCommissionApi(fetcher)
    await expect(api.tree('not-a-brand')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    await expect(api.agent(brand, 'not-an-id')).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.cycle(brand, 'AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA')).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.earnings(brand, 'not-a-cycle')).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.payment(brand, 'AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA')).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.cycles(brand, 0)).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.payments(brand, 101)).rejects.toBeInstanceOf(PlatformApiError)
    await expect(api.tree(brand, 10, 1_000_001)).rejects.toBeInstanceOf(PlatformApiError)
    expect(fetcher).not.toHaveBeenCalled()

    const wrongPage = vi.fn<typeof fetch>().mockResolvedValue(ok({ brand_id: brand, parent_id: null, items: [], limit: 49, offset: 0, total_count: '0' }))
    await expect(createPlatformCommissionApi(wrongPage).tree(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const badPaymentPage = vi.fn<typeof fetch>().mockResolvedValue(ok({ ...page([payment]), offset: 1 }))
    await expect(createPlatformCommissionApi(badPaymentPage).payments(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('rejects foreign scopes, mismatched IDs, invalid enums and unsafe money representations', async () => {
    const foreign = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
    const cases: Array<[unknown, (api: ReturnType<typeof createPlatformCommissionApi>) => Promise<unknown>]> = [
      [{ ...agent, brand_id: foreign }, api => api.agent(brand, agentId)],
      [{ ...agent, path: [foreign] }, api => api.agent(brand, agentId)],
      [{ ...agent, config: { ...agent.config, mode: 'unknown' } }, api => api.agent(brand, agentId)],
      [{ ...cycle, id: foreign }, api => api.cycle(brand, cycleId)],
      [{ ...cycle, state: 'unknown' }, api => api.cycle(brand, cycleId)],
      [{ ...cycle, calendar: { ...cycle.calendar, cycle: 'daily' } }, api => api.cycle(brand, cycleId)],
      [{ ...cycle, version: '2' }, api => api.cycle(brand, cycleId)],
      [{ ...page([{ ...earning, cycle_id: foreign }]), cycle_id: cycleId }, api => api.earnings(brand, cycleId)],
      [{ ...page([{ ...earning, exact_amount: { numerator: huge, denominator: '0' } }]), cycle_id: cycleId }, api => api.earnings(brand, cycleId)],
      [{ ...page([{ ...earning, points: 9007199254740992 }]), cycle_id: cycleId }, api => api.earnings(brand, cycleId)],
      [{ ...payment, payout_mode: 'unknown' }, api => api.payment(brand, paymentId)],
      [{ ...payment, state: 'unknown' }, api => api.payment(brand, paymentId)],
      [{ ...payment, state: 'pending' }, api => api.payment(brand, paymentId)],
      [{ ...payment, total_points: 9007199254740992 }, api => api.payment(brand, paymentId)],
    ]
    for (const [data, call] of cases) {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(data))
      await expect(call(createPlatformCommissionApi(fetcher))).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
    const badTree = vi.fn<typeof fetch>().mockResolvedValue(ok({ brand_id: brand, parent_id: null, items: [{ ...agent, brand_id: foreign }], limit: 50, offset: 0, total_count: '1' }))
    await expect(createPlatformCommissionApi(badTree).tree(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const badCycles = vi.fn<typeof fetch>().mockResolvedValue(ok(page([{ ...cycle, brand_id: foreign }])))
    await expect(createPlatformCommissionApi(badCycles).cycles(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves structured server errors and exposes read-only GET operations only', async () => {
    const denied = new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'Denied' } }), { status: 403 })
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(denied)
    await expect(createPlatformCommissionApi(fetcher).cycles(brand)).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'Denied' })
    expect(fetcher.mock.calls[0]?.[1]?.method).toBe('GET')
  })
})
