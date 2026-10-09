import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { checkCapacityFinancialCore } from './check-capacity-settlement.mjs';

export function checkCapacityWithdrawals(data) {
  const core = checkCapacityFinancialCore(data, 'betting_with_settlement_withdrawals', 49964500);
  checkCapacityWithdrawalFacts(data);
  return { ...core, controlled_withdrawal_load_passed: true, withdrawals: 500, withdrawal_points: 25000 };
}

export function checkCapacityWithdrawalFacts(data) {
  const w = data.withdrawal_report;
  if (!w || [w.planned, w.started, w.completed, w.succeeded, w.status_counts?.['201']].some(n => n !== 500) ||
      w.dropped !== 0 || w.backpressure_dropped !== 0 || Object.keys(w.status_counts).length !== 1 ||
      Object.keys(w.error_counts).length !== 0 || Object.keys(w.code_counts).length !== 1 || w.code_counts[''] !== 500 ||
      data.withdrawal_checks_passed !== true || data.withdrawal_paid_orders !== 500 || data.withdrawal_points !== 25000 ||
      data.withdrawal_paid_during_load !== 500 || data.withdrawal_pending_points !== 0 || data.withdrawal_bad_links !== 0 ||
      data.withdrawal_bad_cycles !== 0 || data.withdrawal_turnover_multiple !== '0.000001') {
    throw new Error('Mixed withdrawal counts, timing, source links or cycle cutoff failed');
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-capacity-withdrawals.mjs REPORT.json');
    console.log(JSON.stringify(checkCapacityWithdrawals(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
