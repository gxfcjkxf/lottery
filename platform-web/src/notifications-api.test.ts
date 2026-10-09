import { describe, expect, it, vi } from 'vitest'
import { PlatformApiError } from './platform-api'
import { createPlatformNotificationsApi } from './notifications-api'

const brand = '8b48d72e-a737-4c11-a54c-c4d96cbcf227'
const event = 'f3cf04e4-9c31-4b6f-9a1d-8a741a6c6012'
const audit = '16d331ed-048c-47df-9e8b-1cae5baef398'
const content = { en: { title: 'Prize update', body: 'You won {points}' }, 'zh-CN': { title: '中奖通知', body: '您获得 {points}' } }
const delivery = { event_id: event, brand_id: brand, status: 'retry', attempt_count: 2, last_error: 'PROVIDER_TIMEOUT', next_attempt_at: '2026-10-10T10:00:00+08:00', sent_at: null }
const template = { brand_id: brand, key: 'bet.order.won', version: 2, content, updated_at: '2026-10-10T02:00:00Z', audit_log_id: audit }
const revision = { id: event, brand_id: brand, key: 'bet.order.won', version: 2, content, changed_by: audit, reason: 'Corrected wording', audit_log_id: audit, created_at: '2026-10-10T02:00:00Z' }
const reply = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })

describe('platform notifications API', () => {
  it('reads deliveries and history with pagination, and templates without a query', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(reply({ items: [delivery] }))
      .mockResolvedValueOnce(reply({ items: [template] }))
      .mockResolvedValueOnce(reply({ items: [revision] }))
    const api = createPlatformNotificationsApi(fetcher)
    await expect(api.deliveries(brand)).resolves.toEqual([delivery])
    await expect(api.templates(brand)).resolves.toEqual([template])
    await expect(api.history(brand, 'bet.order.won')).resolves.toEqual([revision])
    expect(fetcher.mock.calls.map(call => call[0])).toEqual([
      '/api/v1/platform/notification-deliveries?limit=51&offset=0',
      '/api/v1/platform/notification-templates',
      '/api/v1/platform/notification-templates/bet.order.won/history?limit=51&offset=0',
    ])
    for (const [, init] of fetcher.mock.calls) {
      expect(init).toMatchObject({ method: 'GET', credentials: 'same-origin' })
      expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brand)
    }
  })

  it('passes explicit paging values and permits exact server time offsets', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(reply({ items: [delivery] })).mockResolvedValueOnce(reply({ items: [revision] }))
    const api = createPlatformNotificationsApi(fetcher)
    await api.deliveries(brand, 51, 100)
    await api.history(brand, 'bet.order.won', 51, 100)
    expect(fetcher.mock.calls.map(call => call[0])).toEqual([
      '/api/v1/platform/notification-deliveries?limit=51&offset=100',
      '/api/v1/platform/notification-templates/bet.order.won/history?limit=51&offset=100',
    ])
  })

  it('rejects cross-brand/key records, unexpected fields and unknown status values', async () => {
    const cases: [unknown, 'deliveries' | 'templates' | 'history'][] = [
      [{ ...delivery, brand_id: '16d331ed-048c-47df-9e8b-1cae5baef398' }, 'deliveries'],
      [{ ...delivery, status: 'queued' }, 'deliveries'],
      [{ ...delivery, attempt_count: 1.5 }, 'deliveries'],
      [{ ...delivery, extra: true }, 'deliveries'],
      [{ ...template, brand_id: '16d331ed-048c-47df-9e8b-1cae5baef398' }, 'templates'],
      [{ ...template, content: { ...content, fr: { title: 'x', body: 'y' } } }, 'templates'],
      [{ ...revision, key: 'bet.order.placed' }, 'history'],
    ]
    for (const [item, method] of cases) {
      const api = createPlatformNotificationsApi(async () => reply({ items: [item] }))
      const result = method === 'deliveries' ? api.deliveries(brand) : method === 'templates' ? api.templates(brand) : api.history(brand, 'bet.order.won')
      await expect(result).rejects.toBeInstanceOf(PlatformApiError)
      await expect(result).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    }
  })

  it('rejects unsupported keys and unsafe or out-of-range paging before making requests', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformNotificationsApi(fetcher)
    for (const limit of [0, 1.5, Number.MAX_SAFE_INTEGER + 1, 101]) await expect(api.deliveries(brand, limit)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    for (const offset of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, 1_000_001]) await expect(api.history(brand, 'bet.order.won', 51, offset)).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    await expect(api.history(brand, 'bet.order.future')).rejects.toMatchObject({ code: 'INVALID_INPUT' })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('preserves server authorization failures and wraps network failures', async () => {
    const denied = async () => new Response(JSON.stringify({ success: false, error: { code: 'FORBIDDEN', message: 'Forbidden' } }), { status: 403 })
    await expect(createPlatformNotificationsApi(denied).templates(brand)).rejects.toMatchObject({ status: 403, code: 'FORBIDDEN', message: 'Forbidden' })
    await expect(createPlatformNotificationsApi(async () => { throw new TypeError('offline') }).deliveries(brand)).rejects.toMatchObject({ network: true, code: 'NETWORK_ERROR' })
  })

  it('exposes only the three read methods', () => {
    expect(Object.keys(createPlatformNotificationsApi()).sort()).toEqual(['deliveries', 'history', 'templates'])
  })
})
