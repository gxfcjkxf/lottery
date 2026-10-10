import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const sha = value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
function untouched(data) {
  if (!sha(data.original_digest_before) || data.original_digest_before !== data.original_digest_after || data.owned_nodes_stopped !== true) throw new Error('Original database or stopped-node evidence mismatch');
}
function snapshot(value) {
  if (!value?.tables || !sha(value.sha256) || value.migration_count !== 1 || value.tables.schema_migrations?.rows !== 1) throw new Error('Current single-baseline snapshot required');
  const entries = Object.keys(value.tables).sort().map(name => {
    const row = value.tables[name];
    if (!Number.isSafeInteger(row.rows) || row.rows < 0 || !sha(row.sha256)) throw new Error('Invalid table digest');
    return [name, { rows: row.rows, sha256: row.sha256 }];
  });
  if (!entries.length || entries.reduce((sum, [, row]) => sum + row.rows, 0) !== value.row_count ||
      createHash('sha256').update(JSON.stringify(Object.fromEntries(entries))).digest('hex') !== value.sha256) throw new Error('Snapshot table contents or row totals do not match its digest');
  return entries.length;
}
export { snapshot as checkRecoverySnapshot };
export function checkRecoveryBaseline(data) {
  if (data?.schema_version !== 1 || !data.evidence) throw new Error('Recovery evidence required');
  untouched(data);
  const e = data.evidence, tables = snapshot(data.baseline);
  if (snapshot(data.restored) !== tables || data.restored.sha256 !== data.baseline.sha256 || snapshot(data.promoted) !== tables) throw new Error('Restored whole-table snapshot mismatch');
  for (const field of ['replicas', 'streaming_replicas', 'replica_read_only_rejects', 'replica_digest_matches']) if (e[field] !== 2) throw new Error('Two verified replicas required');
  for (const field of ['initial_primary_writable', 'primary_stopped_before_promotion', 'promoted_writable', 'promoted_digest_matches', 'promoted_extra_write_visible', 'remaining_replica_reattached', 'remaining_replica_digest_matches', 'restored_digest_matches', 'restored_immutable_guards_verified', 'original_database_untouched']) if (e[field] !== true) throw new Error(`Missing recovery proof: ${field}`);
  if (e.baseline_table_count !== tables || e.restored_table_count !== tables || e.baseline_row_count !== data.baseline.row_count || e.restored_row_count !== data.restored.row_count ||
      e.migration_count !== 1 || e.restore_migration_count !== 1 || e.expected_migration_count !== 1 || !sha(e.backup_sha256) || !Number.isSafeInteger(e.backup_bytes) || e.backup_bytes <= 0) throw new Error('Recovery counts or backup evidence mismatch');
  for (const field of ['replica_catchup_seconds', 'promotion_seconds', 'restore_seconds']) if (!Number.isFinite(e[field]) || e[field] < 0) throw new Error('Invalid measured recovery duration');
  if (e.automatic_failover_verified !== false || e.production_disaster_recovery_accepted !== false || data.auth_keys_restored !== false || data.pitr_verified !== false || data.application_endpoint_failover_verified !== false) throw new Error('Manual recovery cannot claim automatic or production recovery');
  return { controlled_recovery_passed: true, tables, rows: data.baseline.row_count, production_disaster_recovery_accepted: false };
}
export function checkPhysicalHistory(data) {
  if (data?.schema_version !== 1 || data.replica_nodes !== 2 || data.http_history_routes !== 6 || data.http_successful_history_requests !== 6) throw new Error('Six real history routes and two replicas required');
  untouched(data);
  for (const field of ['primary_write_visible_on_both', 'unallowlisted_route_primary_only', 'paused_replica_rejected', 'both_paused_rejected', 'offline_replica_rejected', 'unavailable_optional_pool_startup', 'wrong_cluster_and_primary_rejected', 'replica_query_error_returned', 'current_template_headers_absent', 'http_audit_failure_no_history_data', 'http_permission_revocation_403', 'http_session_revocation_401', 'primary_fence_permission_error', 'replica_query_timeout_error', 'original_database_untouched']) if (data[field] !== true) throw new Error(`Missing history route proof: ${field}`);
  for (const field of ['round_robin_replica_indexes', 'http_observed_replica_indexes']) if (JSON.stringify(data[field]) !== '[1,2]') throw new Error('Both replica indexes must be observed');
  if (![1, 2].includes(data.notification_route_replica)) throw new Error('Notification history must use a configured replica');
  return { controlled_history_routing_passed: true, replicas: 2, routes: 6 };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const check = { recovery: checkRecoveryBaseline, history: checkPhysicalHistory }[process.argv[2]];
    if (process.argv.length !== 4 || !check) throw new Error('Usage: node scripts/check-recovery-evidence.mjs recovery|history REPORT.json');
    console.log(JSON.stringify(check(JSON.parse(readFileSync(process.argv[3], 'utf8'))), null, 2));
  } catch (error) { console.error(error instanceof Error ? error.message : String(error)); process.exitCode = 1; }
}
