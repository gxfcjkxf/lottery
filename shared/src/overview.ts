export interface OverviewDay {
  day: string
  stake_points: string
  net_points: string
  order_count: string
  bettor_count: string
  refund_points: string
  new_members: string
}
export interface OverviewBrand { brand_id: string; brand_name: string; stake_points: string; net_points: string }
export interface OverviewSummary {
  snapshot_at: string
  timezone: string
  today: string
  total_members: string
  online_members: string
  unsettled_points: string
  available_points: string
  frozen_points: string
  withdrawal_points: string
  withdrawal_count: string
  abnormal_count: string
  failed_settlement_count: string
  cancellation_count: string
  yesterday_same_time_stake: string
  days: OverviewDay[]
  brands?: OverviewBrand[]
}

/** Integer arithmetic preserves precision even when the totals exceed 2^53. */
export function overviewChange(current: string, previous: string): string | null {
  const a = BigInt(current), b = BigInt(previous)
  if (b === 0n) return null
  const tenths = (a - b) * 1000n / b
  const absolute = tenths < 0n ? -tenths : tenths
  return `${tenths < 0n ? '−' : '+'}${absolute / 10n}.${absolute % 10n}%`
}
