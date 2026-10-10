import { PlatformApiError } from './platform-api'

export interface BrandOperation {
  brand_id: string
  version: number
  name: string
  status: 'active' | 'paused'
  updated_at: string
  audit_log_id?: string
}

export interface BrandOperationUpdate {
  version: number
  status: 'active' | 'paused'
  reason: string
}

const BASE = '/api/v1/platform/brand-operation'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const isObject = (value: unknown): value is Record<string, unknown> => Boolean(value && typeof value === 'object' && !Array.isArray(value))
const isText = (value: unknown): value is string => typeof value === 'string'

function invalid(): never {
  throw new PlatformApiError('Invalid brand operation response', 502, 'INVALID_RESPONSE')
}

function brandOperation(value: unknown, brandId: string): BrandOperation {
  if (!isObject(value) || value.brand_id !== brandId || !Number.isSafeInteger(value.version) || Number(value.version) < 1 ||
    !isText(value.name) || (value.status !== 'active' && value.status !== 'paused') ||
    !isText(value.updated_at) || !Number.isFinite(Date.parse(value.updated_at)) ||
    (value.audit_log_id !== undefined && (!isText(value.audit_log_id) || !UUID.test(value.audit_log_id)))) invalid()
  return value as unknown as BrandOperation
}

export function createPlatformBrandOperationApi(fetcher: typeof fetch = fetch) {
  async function request(brandId: string, method: 'GET' | 'PATCH', body?: BrandOperationUpdate, key?: string): Promise<unknown> {
    if (!UUID.test(brandId)) throw new PlatformApiError('A valid brand must be selected', 400, 'BRAND_REQUIRED')
    const headers = new Headers({ Accept: 'application/json', 'X-Brand-ID': brandId })
    if (body !== undefined) headers.set('Content-Type', 'application/json')
    if (key !== undefined) headers.set('Idempotency-Key', key)
    let response: Response
    try {
      response = await fetcher(BASE, {
        method,
        credentials: 'same-origin',
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try {
      envelope = await response.json()
    } catch {
      throw new PlatformApiError('Invalid server response; operation result is unconfirmed', 502, 'INVALID_RESPONSE')
    }
    if (isObject(envelope) && !response.ok && envelope.success === false && isObject(envelope.error) &&
      isText(envelope.error.code) && envelope.error.code.trim() && isText(envelope.error.message) && envelope.error.message.trim()) {
      throw new PlatformApiError(envelope.error.message, response.status, envelope.error.code)
    }
    if (!response.ok || !isObject(envelope) || envelope.success !== true || !isObject(envelope.data) || response.status !== 200) {
      throw new PlatformApiError('Invalid server response; operation result is unconfirmed', 502, 'INVALID_RESPONSE')
    }
    return envelope.data
  }

  return {
    async read(brandId: string): Promise<BrandOperation> {
      return brandOperation(await request(brandId, 'GET'), brandId)
    },
    async update(brandId: string, body: BrandOperationUpdate, key: string): Promise<BrandOperation> {
      if (!Number.isSafeInteger(body.version) || body.version < 1 || (body.status !== 'active' && body.status !== 'paused') || !body.reason.trim()) {
        throw new PlatformApiError('A valid version, status, and reason are required', 400, 'REQUEST_INVALID')
      }
      if (!key.trim()) throw new PlatformApiError('A valid idempotency key is required', 400, 'REQUEST_INVALID')
      const result = brandOperation(await request(brandId, 'PATCH', body, key), brandId)
      if (result.status !== body.status || result.version !== body.version + 1 || !result.audit_log_id) invalid()
      return result
    },
  }
}
