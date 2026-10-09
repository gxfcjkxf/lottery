import { isWithdrawalHistory, isWithdrawalOrder, isWithdrawalPage, type WithdrawalHistory, type WithdrawalOrder, type WithdrawalPage, type WithdrawalState } from '@lottery/shared'
import { PlatformApiError } from './platform-api'

const BASE = '/api/v1/platform/withdrawals'

function invalid(label: string): never {
  throw new PlatformApiError(`Invalid ${label} response`, 0, 'INVALID_RESPONSE')
}

export function createPlatformWithdrawalApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brandId: string): Promise<unknown> {
    if (!brandId) throw new PlatformApiError('A brand must be selected before loading withdrawals', 400, 'BRAND_REQUIRED')
    let response: Response
    try {
      response = await fetcher(path, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
    }
    if (!envelope || typeof envelope !== 'object' || Array.isArray(envelope) || !('success' in envelope) || envelope.success !== true || !('data' in envelope) || !response.ok) {
      const error = envelope && typeof envelope === 'object' && !Array.isArray(envelope) && 'error' in envelope && envelope.error && typeof envelope.error === 'object'
        ? envelope.error as Record<string, unknown> : undefined
      throw new PlatformApiError(typeof error?.message === 'string' ? error.message : `Request failed (${response.status})`, response.status, typeof error?.code === 'string' ? error.code : undefined)
    }
    return envelope.data
  }

  return {
    async list(brandId: string, filters: { state?: WithdrawalState; memberId?: string; limit: number; offset: number }): Promise<WithdrawalPage> {
      if (!brandId) throw new PlatformApiError('A brand must be selected before loading withdrawals', 400, 'BRAND_REQUIRED')
      const query = new URLSearchParams()
      if (filters.state !== undefined) query.set('state', filters.state)
      if (filters.memberId?.trim()) query.set('member_id', filters.memberId.trim())
      query.set('limit', String(filters.limit))
      query.set('offset', String(filters.offset))
      const value = await get(`${BASE}?${query}`, brandId)
      if (!isWithdrawalPage(value)) invalid('withdrawal page')
      const filteredMember = filters.memberId?.trim()
      if (value.brand_id !== brandId || value.limit !== filters.limit || value.offset !== filters.offset || value.items.length > filters.limit ||
        value.items.some(item => item.brand_id !== brandId || (filteredMember !== undefined && filteredMember !== '' && item.member_id !== filteredMember) || (filters.state !== undefined && item.state !== filters.state))) invalid('withdrawal page')
      return value
    },
    async read(brandId: string, orderId: string): Promise<WithdrawalOrder> {
      const value = await get(`${BASE}/${encodeURIComponent(orderId)}`, brandId)
      if (!isWithdrawalOrder(value) || value.brand_id !== brandId || value.id !== orderId) invalid('withdrawal order')
      return value
    },
    async history(brandId: string, orderId: string): Promise<WithdrawalHistory> {
      const value = await get(`${BASE}/${encodeURIComponent(orderId)}/history`, brandId)
      if (!isWithdrawalHistory(value) || value.brand_id !== brandId || value.order_id !== orderId) invalid('withdrawal history')
      return value
    },
  }
}
