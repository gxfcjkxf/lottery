import { describe, expect, it, vi } from 'vitest'
import { createPlatformBrandOperationApi, type BrandOperation } from './brand-operation-api'

const brandId = '4b1bc4f5-24ed-456b-8c43-680bcb710c51'
const auditId = '5b698c19-f51e-48cc-8d63-bd9d87e45cf7'
const record: BrandOperation = {
  brand_id: brandId,
  version: 3,
  name: 'Harbor',
  status: 'paused',
  updated_at: '2026-10-09T01:02:03Z',
  audit_log_id: auditId,
}

function reply(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('platform brand operation API', () => {
  it('reads fresh status and version under the explicit brand scope', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(record))
    await expect(createPlatformBrandOperationApi(fetcher).read(brandId)).resolves.toEqual(record)
    const [url, init] = fetcher.mock.calls[0]!
    expect(url).toBe('/api/v1/platform/brand-operation')
    expect(init?.method).toBe('GET')
    expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brandId)
    expect(init?.credentials).toBe('same-origin')
  })

  it('accepts the original request receipt at exactly the next version with audit evidence', async () => {
    const body = { version: 3, status: 'active' as const, reason: 'Resume after review' }
    const updated = { ...record, version: 4, status: 'active' as const }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(reply(updated))
    await expect(createPlatformBrandOperationApi(fetcher).update(brandId, body, 'fixed-key-123')).resolves.toEqual(updated)
    const [url, init] = fetcher.mock.calls[0]!
    expect(url).toBe('/api/v1/platform/brand-operation')
    expect(init?.method).toBe('PATCH')
    expect(new Headers(init?.headers).get('X-Brand-ID')).toBe(brandId)
    expect(new Headers(init?.headers).get('Idempotency-Key')).toBe('fixed-key-123')
    expect(JSON.parse(String(init?.body))).toEqual(body)
  })

  it('rejects an invalid brand scope or malformed and mismatched records', async () => {
    await expect(createPlatformBrandOperationApi().read('not-a-brand')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    await expect(createPlatformBrandOperationApi(async () => reply({ ...record, brand_id: auditId })).read(brandId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformBrandOperationApi(async () => reply({ ...record, status: 'disabled' })).read(brandId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformBrandOperationApi(async () => reply({ ...record, status: 'paused' })).update(brandId, { version: 3, status: 'active', reason: 'Resume' }, 'fixed-key-123')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires an audit ID and exactly request version plus one on PATCH receipts', async () => {
    const body = { version: 3, status: 'active' as const, reason: 'Resume after review' }
    const missingAudit = { ...record, version: 4, status: 'active' as const, audit_log_id: undefined }
    const wrongVersion = { ...record, version: 5, status: 'active' as const }
    await expect(createPlatformBrandOperationApi(async () => reply(missingAudit)).update(brandId, body, 'fixed-key-123')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformBrandOperationApi(async () => reply(wrongVersion)).update(brandId, body, 'fixed-key-123')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('preserves structured permission errors and marks malformed responses unconfirmed', async () => {
    const denied = async () => new Response(JSON.stringify({ success: false, error: { code: 'FORBIDDEN', message: 'Permission denied' } }), { status: 403 })
    await expect(createPlatformBrandOperationApi(denied).read(brandId)).rejects.toMatchObject({ status: 403, code: 'FORBIDDEN', message: 'Permission denied' })
    const malformed = async () => new Response('not json', { status: 400 })
    await expect(createPlatformBrandOperationApi(malformed).update(brandId, { version: 3, status: 'active', reason: 'Resume' }, 'fixed-key-123')).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
  })

  it('requires a reason and key before a write', async () => {
    const fetcher = vi.fn<typeof fetch>()
    const api = createPlatformBrandOperationApi(fetcher)
    await expect(api.update(brandId, { version: 3, status: 'active', reason: '  ' }, 'fixed-key-123')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.update(brandId, { version: 3, status: 'active', reason: 'Resume' }, ' ')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    expect(fetcher).not.toHaveBeenCalled()
  })
})
