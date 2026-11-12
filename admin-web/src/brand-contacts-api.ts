import { AdminApiError, readAdminEnvelope } from './admin-api'
import { isBrandContactConfig, contactAssetPattern, type BrandContactConfig } from '../../shared/src/brand-contacts'
export interface BrandContactsRecord { brand_id: string; version: number; status: string; config: BrandContactConfig; audit_log_id?: string }
export interface BrandContactsBody { version: number; config: BrandContactConfig }
export function createBrandContactsApi(fetcher: typeof fetch = fetch) {
  async function request(brand: string, method: string, body?: BrandContactsBody, key?: string): Promise<BrandContactsRecord> {
    const headers = new Headers({ 'X-Brand-ID': brand })
    if (body) headers.set('Content-Type', 'application/json')
    if (key) headers.set('Idempotency-Key', key)
    const response = await fetcher('/api/v1/admin/brand-contacts', { method, credentials: 'same-origin', headers, ...(body ? { body: JSON.stringify(body) } : {}) })
    const { data } = await readAdminEnvelope(response)
    const value = data as BrandContactsRecord
    if (response.status !== 200 || !value || value.brand_id !== brand || !Number.isSafeInteger(value.version) || value.version < 1 || !['active', 'paused', 'disabled'].includes(value.status) || !isBrandContactConfig(value.config) || body && value.version !== body.version + 1) throw new AdminApiError('Invalid contact response', 502, 'INVALID_RESPONSE')
    return value
  }
  return {
    get: (brand: string) => request(brand, 'GET'),
    put: (brand: string, body: BrandContactsBody, key: string) => request(brand, 'PUT', body, key),
    async upload(brand: string, file: File) {
      const response = await fetcher('/api/v1/admin/brand-contacts/image', { method: 'POST', credentials: 'same-origin', headers: { 'X-Brand-ID': brand, 'Content-Type': file.type }, body: file })
      const { data } = await readAdminEnvelope(response)
      const url = (data as { url?: unknown })?.url
      if (response.status !== 200 || typeof url !== 'string' || !contactAssetPattern.test(url)) throw new AdminApiError('Invalid image receipt', 502, 'INVALID_RESPONSE')
      return url
    },
  }
}
