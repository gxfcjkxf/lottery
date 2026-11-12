import { isPoints, pointsMinor } from '@lottery/shared'
import { AdminApiError, readAdminEnvelope } from './admin-api'
const isPositivePoints=(value:string)=>isPoints(value)&&pointsMinor(value)>0n

export interface BetLimitGame { id: string; name: string; configured: boolean }
export interface BetLimits {
  brand_id: string; game_id: string; version: number
  min_points: string; max_points: string; max_period_points: string; max_user_period_points: string
}
export const limitFields = ['min_points', 'max_points', 'max_period_points', 'max_user_period_points'] as const
export function validBetLimits(value: unknown): value is BetLimits {
  if (!value || typeof value !== 'object') return false
  const v = value as BetLimits
  return typeof v.brand_id === 'string' && typeof v.game_id === 'string' && Number.isSafeInteger(v.version) && v.version > 0 &&
    limitFields.every(key => typeof v[key] === 'string' && isPositivePoints(v[key])) &&
    pointsMinor(v.max_points) >= pointsMinor(v.min_points) && pointsMinor(v.max_period_points) >= pointsMinor(v.max_points) && pointsMinor(v.max_user_period_points) >= pointsMinor(v.max_points)
}
export function createBetLimitsApi(fetcher: typeof fetch = fetch) {
  async function request(brandId: string, path: string, body?: BetLimits, key?: string): Promise<unknown> {
    const headers = new Headers({ Accept: 'application/json', 'X-Brand-ID': brandId })
    if (body) { headers.set('Content-Type', 'application/json'); headers.set('Idempotency-Key', key!) }
    let response: Response
    try { response = await fetcher(`/api/v1/admin${path}`, { method: body ? 'POST' : 'GET', credentials: 'same-origin', headers, ...(body ? { body: JSON.stringify(body) } : {}) }) }
    catch { throw new AdminApiError('Network failure', 0, 'NETWORK_ERROR') }
    return (await readAdminEnvelope<unknown>(response)).data
  }
  async function policy(brandId: string, gameId: string, body?: BetLimits, key?: string): Promise<BetLimits> {
    const value = await request(brandId, `/platform-games/${encodeURIComponent(gameId)}/bet-limits`, body, key)
    if (!validBetLimits(value) || value.brand_id !== brandId || value.game_id !== gameId) throw new AdminApiError('Invalid limits response', 502, 'INVALID_RESPONSE')
    return value
  }
  return {
    async games(brandId: string): Promise<BetLimitGame[]> {
      const value = await request(brandId, '/bet-limits') as { items?: unknown }
      if (!value || !Array.isArray(value.items) || !value.items.every(game => game && typeof game.id === 'string' && typeof game.name === 'string' && typeof game.configured === 'boolean')) throw new AdminApiError('Invalid limits catalog', 502, 'INVALID_RESPONSE')
      return value.items
    },
    read: (brandId: string, gameId: string) => policy(brandId, gameId),
    save: (body: BetLimits, key: string) => policy(body.brand_id, body.game_id, body, key),
  }
}
