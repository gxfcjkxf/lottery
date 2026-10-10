import { describe, expect, it, vi } from 'vitest'
import { createPlatformOperationalPoliciesApi } from './operational-policies-api'

const brand = '11111111-1111-4111-8111-111111111111'
const otherBrand = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const audit = '22222222-2222-4222-8222-222222222222'
const at = '2026-10-10T08:30:00Z'
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status })
const current = [
  { brand_id: brand, version: 1, config: { min_bet_points: '1', max_bet_points: null, max_period_points: null, max_user_period_points: null, user_cancel_allowed: false }, updated_at: at },
  { brand_id: brand, version: 1, mode: null, updated_at: at },
  { brand_id: brand, version: 1, config: { enabled: false, max_depth: 5, ratio_cap: '0', mode: 'loss', cycle: 'monthly' }, updated_at: at },
  { brand_id: brand, version: 1, config: { age_enabled: false, minimum_age: null, region_enabled: false, allowed_countries: [], identity_enabled: false, account_risk_enabled: false, betting_risk_enabled: false, exclusion_enabled: false, responsible_gambling_enabled: false }, updated_at: at },
  { brand_id: brand, version: 1, enabled: false, audit_log_id: '', updated_at: at },
  { brand_id: brand, version: 1, enabled: false, audit_log_id: '', updated_at: at },
] as const
const paths = ['/bet-policy', '/settlement-policy', '/agent-policy', '/compliance-policy', '/commission-payment-policy', '/commission-correction-policy']
const methods = ['betting', 'settlement', 'agents', 'compliance', 'payment', 'correction'] as const

describe('platform operational policy read API', () => {
  it('reads all six current initial policy DTOs using brand-scoped GET requests only', async () => {
    const fetcher = vi.fn<typeof fetch>()
    for (const data of current) fetcher.mockResolvedValueOnce(ok(data))
    const api = createPlatformOperationalPoliciesApi(fetcher)
    expect(Object.keys(api)).toEqual(methods)
    for (let i = 0; i < methods.length; i++) await expect(api[methods[i]](brand)).resolves.toEqual(current[i])
    expect(fetcher.mock.calls.map(call => call[0])).toEqual(paths.map(path => `/api/v1/platform${path}`))
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.method).toBe('GET')
      expect(init?.credentials).toBe('same-origin')
      expect(new Headers(init?.headers).get('Accept')).toBe('application/json')
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
      expect(init?.body).toBeUndefined()
    }
  })

  it('accepts updated versions, nondefault settings, null settlement mode, and audited payment switches', async () => {
    const updated = [
      { ...current[0], version: 4, config: { min_bet_points: '25', max_bet_points: '9007199254740993', max_period_points: null, max_user_period_points: '9223372036854775807', user_cancel_allowed: true } },
      { ...current[1], version: 2, mode: 'manual', audit_log_id: audit },
      { ...current[2], version: 3, config: { enabled: true, max_depth: 3, ratio_cap: '0.125', mode: 'turnover', cycle: 'weekly' } },
      { ...current[3], version: 2, audit_log_id: audit, config: { age_enabled: true, minimum_age: 21, region_enabled: true, allowed_countries: ['SG', 'US'], identity_enabled: true, account_risk_enabled: true, betting_risk_enabled: false, exclusion_enabled: true, responsible_gambling_enabled: false } },
      { ...current[4], version: 2, enabled: true, audit_log_id: audit },
      { ...current[5], version: 2, enabled: true, audit_log_id: audit },
    ]
    const fetcher = vi.fn<typeof fetch>()
    for (const data of updated) fetcher.mockResolvedValueOnce(ok(data))
    const api = createPlatformOperationalPoliciesApi(fetcher)
    for (let i = 0; i < methods.length; i++) await expect(api[methods[i]](brand)).resolves.toEqual(updated[i])
  })

  it('rejects invalid brands and malformed policy fields without making an invalid-brand request', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(current[0]))
    const api = createPlatformOperationalPoliciesApi(fetcher)
    await expect(api.betting('bad-brand')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    expect(fetcher).not.toHaveBeenCalled()
    const cases: Array<[unknown, (client: ReturnType<typeof createPlatformOperationalPoliciesApi>) => Promise<unknown>]> = [
      [{ ...current[0], brand_id: otherBrand }, client => client.betting(brand)],
      [{ ...current[0], audit_log_id: audit }, client => client.betting(brand)],
      [{ ...current[1], audit_log_id: '' }, client => client.settlement(brand)],
      [{ ...current[0], config: { ...current[0].config, min_bet_points: '9007199254740994', max_bet_points: '9007199254740993' } }, client => client.betting(brand)],
      [{ ...current[0], config: { ...current[0].config, user_cancel_allowed: 'false' } }, client => client.betting(brand)],
      [{ ...current[1], mode: 'mixed' }, client => client.settlement(brand)],
      [{ ...current[2], config: { ...current[2].config, max_depth: 33 } }, client => client.agents(brand)],
      [{ ...current[2], config: { ...current[2].config, ratio_cap: '0.1234567' } }, client => client.agents(brand)],
      [{ ...current[3], config: { ...current[3].config, age_enabled: true } }, client => client.compliance(brand)],
      [{ ...current[3], config: { ...current[3].config, identity_enabled: true, allowed_countries: null } }, client => client.compliance(brand)],
      [{ ...current[3], config: { ...current[3].config, allowed_countries: ['US', 'SG'] } }, client => client.compliance(brand)],
      ...(['account_risk_enabled', 'betting_risk_enabled', 'exclusion_enabled', 'responsible_gambling_enabled'] as const).flatMap(key => [
        [{ ...current[3], config: Object.fromEntries(Object.entries(current[3].config).filter(([field]) => field !== key)) }, client => client.compliance(brand)] as [unknown, (client: ReturnType<typeof createPlatformOperationalPoliciesApi>) => Promise<unknown>],
        [{ ...current[3], config: { ...current[3].config, [key]: 'false' } }, client => client.compliance(brand)] as [unknown, (client: ReturnType<typeof createPlatformOperationalPoliciesApi>) => Promise<unknown>],
      ]),
      [{ ...current[4], audit_log_id: null }, client => client.payment(brand)],
      [{ ...current[4], version: 1, enabled: true, audit_log_id: audit }, client => client.payment(brand)],
      [{ ...current[5], version: 2, audit_log_id: '' }, client => client.correction(brand)],
    ]
    for (const [data, call] of cases) {
      const bad = vi.fn<typeof fetch>().mockResolvedValue(ok(data))
      await expect(call(createPlatformOperationalPoliciesApi(bad))).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })

  it('preserves access denial and rejects success payloads with a non-200 status', async () => {
    const denied = new Response(JSON.stringify({ success: false, error: { code: 'PERMISSION_DENIED', message: 'Denied' } }), { status: 403 })
    await expect(createPlatformOperationalPoliciesApi(vi.fn<typeof fetch>().mockResolvedValue(denied)).settlement(brand))
      .rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'Denied' })
    await expect(createPlatformOperationalPoliciesApi(vi.fn<typeof fetch>().mockResolvedValue(ok(current[0], 201))).betting(brand))
      .rejects.toMatchObject({ status: 201, code: 'INVALID_RESPONSE' })
  })
})
