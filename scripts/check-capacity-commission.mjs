import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { checkCapacityFinancialCore } from './check-capacity-settlement.mjs';
import { checkCapacityWithdrawalFacts } from './check-capacity-withdrawals.mjs';

export function checkCapacityCommission(data) {
  const core = checkCapacityFinancialCore(data, 'betting_with_settlement_withdrawals_commission', 49964550);
  checkCapacityWithdrawalFacts(data);
  if (data.commission_ready_cycles !== 2 || data.commission_paid_payments !== 2 ||
      data.commission_entries !== 2 || data.commission_points !== 50 || data.commission_credits_during_load !== 2 ||
      data.commission_bad_links !== 0 || data.commission_cycle_targets !== 500 ||
      data.commission_out_of_window_orders !== 0 || data.commission_checks_passed !== true ||
      !Number.isFinite(Date.parse(data.commission_boundary_utc)) ||
      Date.parse(data.commission_boundary_utc) > Date.parse(data.load_started_at_utc)) {
    throw new Error('Commission cycle, actual payout, boundary or ledger linkage failed');
  }
  return { ...core, controlled_commission_load_passed: true, withdrawals: 500, commission_points: 50 };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-capacity-commission.mjs REPORT.json');
    console.log(JSON.stringify(checkCapacityCommission(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
