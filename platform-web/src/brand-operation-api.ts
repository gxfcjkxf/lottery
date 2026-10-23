import { readPlatformEnvelope } from './platform-api'

import { PlatformApiError } from './platform-api'

export interface BrandOperation {
  brand_id: string
  version: number
  name: string
  status: 'active' | 'paused' | 'disabled'
  pause_starts_at?: string
  pause_ends_at?: string
  updated_at: string
  audit_log_id?: string
}

export interface BrandOperationUpdate {
  version: number
  status: 'active' | 'paused' | 'disabled'
  reason: string
  pause_starts_at?: string
  pause_ends_at?: string
}

const BASE = '/api/v1/platform/brand-operation'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const isObject = (value: unknown): value is Record<string, unknown> => Boolean(value && typeof value === 'object' && !Array.isArray(value))
const isText = (value: unknown): value is string => typeof value === 'string'
const sameInstant = (left?: string, right?: string) => left === right || (left !== undefined && right !== undefined && Date.parse(left) === Date.parse(right))

function invalid(): never {
  throw new PlatformApiError('Invalid brand operation response', 502, 'INVALID_RESPONSE')
}

function brandOperation(value: unknown, brandId: string): BrandOperation {
  if (!isObject(value) || value.brand_id !== brandId || !Number.isSafeInteger(value.version) || Number(value.version) < 1 ||
    !isText(value.name) || (value.status !== 'active' && value.status !== 'paused' && value.status !== 'disabled') ||
    !isText(value.updated_at) || !Number.isFinite(Date.parse(value.updated_at)) ||
    (value.audit_log_id !== undefined && (!isText(value.audit_log_id) || !UUID.test(value.audit_log_id)))) invalid()
  if ((value.pause_starts_at === undefined) !== (value.pause_ends_at === undefined)) invalid()
  if (value.pause_starts_at !== undefined && (typeof value.pause_starts_at !== 'string' || typeof value.pause_ends_at !== 'string' || !Number.isFinite(Date.parse(value.pause_starts_at)) || !(Date.parse(value.pause_ends_at) > Date.parse(value.pause_starts_at)) || value.status === 'disabled')) invalid()
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
    const envelope = await readPlatformEnvelope<unknown>(response);
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
      if (!Number.isSafeInteger(body.version) || body.version < 1 || (body.status !== 'active' && body.status !== 'paused' && body.status !== 'disabled')) {
        throw new PlatformApiError('A valid version and status are required', 400, 'REQUEST_INVALID')
      }
      if (Array.from(body.reason).length > 500) throw new PlatformApiError('A reason of up to 500 characters is required', 400, 'REQUEST_INVALID')
      if (!key.trim()) throw new PlatformApiError('A valid idempotency key is required', 400, 'REQUEST_INVALID')
      const result = brandOperation(await request(brandId, 'PATCH', body, key), brandId)
      const statusMatches = result.status === body.status || (body.status === 'paused' && body.pause_starts_at !== undefined && result.status === 'active')
      if (!statusMatches || !sameInstant(result.pause_starts_at, body.pause_starts_at) ||
        !sameInstant(result.pause_ends_at, body.pause_ends_at) || result.version !== body.version + 1 || !result.audit_log_id) invalid()
      return result
    },
  }
}
