import { describe, it, expect } from 'vitest'
import { createBetLimitsApi, validBetLimits } from './bet-limits-api'
const record = { brand_id: 'brand-a', game_id: 'game-a', version: 3, min_points: '1', max_points: '100', max_period_points: '1000', max_user_period_points: '500' }
const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }))
describe('platform game betting limits', () => {
  it('requires exact integer amounts and meaningful limit relationships', () => {
    expect(validBetLimits(record)).toBe(true)
    for (const change of [{ min_points: 1 }, { min_points: '0' }, { max_points: '0.5' }, { max_points: '9223372036854775808' }, { min_points: '101' }, { max_user_period_points: '99' }, { max_period_points: '99' }]) expect(validBetLimits({ ...record, ...change })).toBe(false)
  })
  it('reads the actual platform-game catalog and scopes every request by brand', async () => {
    const calls: Array<[string, RequestInit | undefined]> = []
    const api = createBetLimitsApi((async (url, options) => { calls.push([String(url), options]); return ok({ items: [{ id: 'game-a', name: 'Mark Six', configured: true }] }) }) as typeof fetch)
    expect(await api.games('brand-a')).toEqual([{ id: 'game-a', name: 'Mark Six', configured: true }])
    expect(calls[0]![0]).toBe('/api/v1/admin/bet-limits')
    expect(new Headers(calls[0]![1]!.headers).get('X-Brand-ID')).toBe('brand-a')
  })
  it('never accepts another brand or game in the response', async () => {
    const api = createBetLimitsApi((async () => ok({ ...record, brand_id: 'brand-b' })) as typeof fetch)
    await expect(api.read('brand-a', 'game-a')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })
  it('requires an explicit configured state from the catalog', async () => {
    const api = createBetLimitsApi((async () => ok({ items: [{ id: 'game-a', name: 'Mark Six' }] })) as typeof fetch)
    await expect(api.games('brand-a')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })
  it('sends only the limits snapshot with the supplied idempotency key', async () => {
    let body: unknown, headers: Headers, method: string | undefined
    const api = createBetLimitsApi((async (_url, options) => { body = JSON.parse(String(options!.body)); headers = new Headers(options!.headers); method = options!.method; return ok({ ...record, version: 4 }) }) as typeof fetch)
    expect((await api.save(record, 'same-key')).version).toBe(4)
    expect(body).toEqual(record); expect(headers!.get('Idempotency-Key')).toBe('same-key'); expect(method).toBe('POST')
  })
  it('rejects malformed successful writes so their receipts remain unconfirmed', async () => {
    const api = createBetLimitsApi((async () => ok({ ...record, version: '4' })) as typeof fetch)
    await expect(api.save(record, 'same-key')).rejects.toMatchObject({ code: 'INVALID_RESPONSE', status: 502 })
  })
})
