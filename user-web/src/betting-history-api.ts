import { readPaginationTotal } from '@lottery/shared'
import { createApiClient } from '../../shared/src/api'
import type { ConfiguredLiveOrder } from '@lottery/shared'

interface HistoryBase { id: string; game_id: string; game_name: string; period_no: string; placed_at: string }
export type HistoryItem = HistoryBase & { kind: 'configured'; order: ConfiguredLiveOrder }
export interface HistoryPage { total_count: string;
  brand_id: string; member_id: string; game_id: string
  games: Array<{ id: string; name: string }>; items: HistoryItem[]
  limit: number; next_cursor: string | null
}
export interface HistoryScope { brand_id: string; member_id: string }
const decimal = (value: unknown): value is string => typeof value === 'string' && /^(0|[1-9]\d*)$/.test(value)
export async function readBettingHistory(brandCode: string | undefined, scope: HistoryScope, gameId = '', cursor: string | null = null, signal?: AbortSignal, limit = 20): Promise<HistoryPage> {
  const query = new URLSearchParams({ limit: String(limit) })
  if (gameId) query.set('game_id', gameId)
  if (cursor) query.set('cursor', cursor)
  const base = `/api/v1${brandCode ? `/b/${encodeURIComponent(brandCode)}` : ''}`
  const page = await createApiClient({ brandCode }).request<HistoryPage>(`${base}/betting-history?${query}`, { signal })
  if (!page || page.brand_id !== scope.brand_id || page.member_id !== scope.member_id || page.game_id !== gameId || page.limit !== limit ||
    !(page.next_cursor === null || typeof page.next_cursor === 'string' && page.next_cursor.length > 0) ||
    !Array.isArray(page.games) || page.games.some(g => !g || typeof g.id !== 'string' || !g.id || typeof g.name !== 'string' || !g.name.trim()) ||
    !Array.isArray(page.items) || page.items.length > limit) throw new Error('Invalid betting history response')
  const ids = new Set<string>()
  for (const item of page.items) {
    if (!item || item.kind !== 'configured' || typeof item.id !== 'string' || !item.id ||
      typeof item.game_name !== 'string' || !item.game_name.trim() || typeof item.period_no !== 'string' || !item.period_no ||
      !Number.isFinite(Date.parse(item.placed_at)) || !page.games.some(g => g.id === item.game_id && g.name === item.game_name) ||
      gameId && item.game_id !== gameId || !item.order || item.order.id !== item.id || item.order.game_id !== item.game_id ||
      ids.has(`${item.kind}:${item.id}`)) throw new Error('Invalid betting history item')
    ids.add(`${item.kind}:${item.id}`)
    if (item.kind === 'configured') {
      if (!item.order.snapshot || !item.order.quote || !Array.isArray(item.order.quote.lines) || !decimal(item.order.quote.bet_points)) throw new Error('Invalid configured history item')
    }
  }
  readPaginationTotal(page)
  return page
}
