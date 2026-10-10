import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { checkRecoverySnapshot } from './check-recovery-evidence.mjs';

export function checkPITREvidence(data) {
  if (data?.schema_version !== 1 || data.profile !== 'named_restore_point_pitr' || data.restore_point_name !== 'lottery_pitr_target' ||
      typeof data.restore_point_lsn !== 'string' || !/^[0-9A-F]+\/[0-9A-F]+$/.test(data.restore_point_lsn)) throw new Error('Named isolated PITR profile required');
  for (const field of ['basebackup_before_restore_point', 'target_wal_archived', 'archive_checksum_verified', 'post_target_write_excluded',
    'whole_table_target_digest_matches', 'primary_stopped_before_recovery', 'recovered_readonly_at_target', 'recovered_promoted_writable',
    'restored_immutable_guards_verified', 'original_database_untouched', 'owned_nodes_stopped', 'pitr_verified']) {
    if (data[field] !== true) throw new Error(`Missing PITR proof: ${field}`);
  }
  const tables = checkRecoverySnapshot(data.target);
  for (const field of ['baseline', 'post_target', 'restored']) if (checkRecoverySnapshot(data[field]) !== tables) throw new Error('PITR schema coverage mismatch');
  if (data.restored.sha256 !== data.target.sha256 || data.post_target.sha256 === data.target.sha256 || data.baseline.sha256 === data.target.sha256 ||
      data.before_target_gift_points !== 123 || data.target_gift_points !== 130 || data.post_target_primary_gift_points !== 180 || data.recovered_gift_points !== 130) throw new Error('PITR cutoff or full target snapshot mismatch');
  if (!/^[a-f0-9]{64}$/.test(data.original_digest_before) || data.original_digest_after !== data.original_digest_before) throw new Error('Original database changed');
  if (!Array.isArray(data.archive_files) || !data.archive_files.length) throw new Error('Missing actual WAL archive manifest');
  const names = new Set();
  for (const file of data.archive_files) {
    if (typeof file.file !== 'string' || !/^[A-Za-z0-9.]+$/.test(file.file) || names.has(file.file) || !Number.isSafeInteger(file.bytes) || file.bytes <= 0 || !/^[a-f0-9]{64}$/.test(file.sha256)) throw new Error('Invalid archive manifest entry');
    names.add(file.file);
  }
  if (data.automatic_failover_verified !== false || data.production_disaster_recovery_accepted !== false || data.auth_keys_restored !== false || !Number.isFinite(data.restore_seconds) || data.restore_seconds < 0) throw new Error('Isolated PITR cannot claim automatic or production recovery');
  return { controlled_named_pitr_passed: true, tables, recovered_gift_points: 130, post_target_write_excluded: true, production_disaster_recovery_accepted: false };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    if (process.argv.length !== 3) throw new Error('Usage: node scripts/check-pitr-evidence.mjs REPORT.json');
    console.log(JSON.stringify(checkPITREvidence(JSON.parse(readFileSync(process.argv[2], 'utf8'))), null, 2));
  } catch (error) { console.error(error instanceof Error ? error.message : String(error)); process.exitCode = 1; }
}
