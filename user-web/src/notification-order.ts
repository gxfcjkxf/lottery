import { createApiClient } from '../../shared/src/api'
import { configuredTicketLabel, configuredTicketPrices, parseConfiguredSnapshot, type ConfiguredLiveOrder } from '@lottery/shared'

export async function readNotificationOrder(brandCode: string | undefined, scope: { brand_id: string; member_id: string }, id: string): Promise<ConfiguredLiveOrder> {
  const base = `/api/v1${brandCode ? `/b/${encodeURIComponent(brandCode)}` : ''}`
  const result = await createApiClient({ brandCode }).request<{ brand_id: string; member_id: string; order: ConfiguredLiveOrder }>(`${base}/configured/orders/${encodeURIComponent(id)}`)
  const order = result?.order
  if (result?.brand_id !== scope.brand_id || result.member_id !== scope.member_id || !order || order.id !== id ||
    typeof order.game_name !== 'string' || !order.game_name.trim() || typeof order.period_no !== 'string' || !order.period_no ||
    !order.quote || !Array.isArray(order.quote.lines) || !order.quote.lines.length || !/^[1-9]\d*$/.test(order.quote.bet_points)) {
    throw new Error('Invalid notification order response')
  }
  order.snapshot = parseConfiguredSnapshot(JSON.stringify(order.snapshot))
  for (const line of order.quote.lines) {
    if (!/^[1-9]\d*$/.test(line.stake)) throw new Error('Invalid notification order stake')
    // Resolve every line in both supported languages before rendering a ticket.
    for (const locale of ['en', 'zh-CN'] as const) {
      configuredTicketLabel(order.snapshot, line, locale)
      configuredTicketPrices(order.snapshot, line, locale)
    }
  }
  return order
}
