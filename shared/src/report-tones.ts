import { pointsMinor } from './points'

export type NumericTone = 'info' | 'success' | 'warning' | 'danger' | 'muted'

const reportTones: Readonly<Partial<Record<string, NumericTone>>> = {
  won_count: 'success',
  current_prize_points: 'success',
  prize_credit_points: 'success',
  refund_points: 'success',
  available_points: 'success',
  recharge_points: 'success',
  paid_count: 'success',
  paid_points: 'success',
  paid_entry_count: 'success',
  grant_entry_count: 'success',
  grant_points: 'success',
  granted_count: 'success',
  granted_points: 'success',
  adjustment_credit_points: 'success',
  correction_credit_points: 'success',

  unfinalized_stake_points: 'warning',
  unsettled_points: 'warning',
  correction_open_count: 'warning',
  withdrawal_points: 'warning',
  reviewing_count: 'warning',
  reviewing_points: 'warning',
  processing_count: 'warning',
  processing_points: 'warning',

  lost_count: 'danger',
  final_lost_stake_points: 'danger',
  abnormal_count: 'danger',
  abnormal_stake_points: 'danger',
  cancelled_count: 'danger',
  cancelled_points: 'danger',
  prize_reversal_points: 'danger',
  rejected_count: 'danger',
  rejected_points: 'danger',
  failed_count: 'danger',
  failed_points: 'danger',
  reversal_entry_count: 'danger',
  reversal_points: 'danger',
  revoked_count: 'danger',
  revoked_points: 'danger',
  adjustment_debit_points: 'danger',
  correction_debit_points: 'danger',

  frozen_points: 'muted',
}

// Use raw decimal strings so display grouping and large amounts cannot change the sign.
export function signedPointsTone(value: string): NumericTone {
  const minor = pointsMinor(value)
  return minor < 0n ? 'danger' : minor > 0n ? 'success' : 'info'
}

export function reportValueTone(field: string, value: string): NumericTone {
  if (field === 'net_points') return signedPointsTone(value)
  return reportTones[field] ?? 'info'
}
