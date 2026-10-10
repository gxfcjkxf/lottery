import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { checkPITREvidence } from '../../scripts/check-pitr-evidence.mjs';

const hash = text => createHash('sha256').update(text).digest('hex');
function snapshot(points) {
  const tables = { point_buckets: { rows: 1, sha256: hash(String(points)) }, schema_migrations: { rows: 1, sha256: hash('current baseline') } };
  return { tables, row_count: 2, migration_count: 1, sha256: hash(JSON.stringify(tables)) };
}
function report() {
  return {
    schema_version: 1, profile: 'named_restore_point_pitr', restore_point_name: 'lottery_pitr_target', restore_point_lsn: '0/4000028',
    baseline: snapshot(123), target: snapshot(130), post_target: snapshot(180), restored: snapshot(130),
    before_target_gift_points: 123, target_gift_points: 130, post_target_primary_gift_points: 180, recovered_gift_points: 130,
    basebackup_before_restore_point: true, target_wal_archived: true, archive_checksum_verified: true, post_target_write_excluded: true,
    whole_table_target_digest_matches: true, primary_stopped_before_recovery: true, recovered_readonly_at_target: true,
    recovered_promoted_writable: true, restored_immutable_guards_verified: true, original_database_untouched: true, owned_nodes_stopped: true, pitr_verified: true,
    archive_files: [{ file: '000000010000000000000004', bytes: 16777216, sha256: hash('synthetic WAL') }],
    original_digest_before: hash('original'), original_digest_after: hash('original'), automatic_failover_verified: false,
    production_disaster_recovery_accepted: false, auth_keys_restored: false, restore_seconds: 1,
  };
}
test('named PITR evidence stops at the target rather than the later committed state', () => {
  assert.deepEqual(checkPITREvidence(report()), { controlled_named_pitr_passed: true, tables: 2, recovered_gift_points: 130, post_target_write_excluded: true, production_disaster_recovery_accepted: false });
});
test('restoring the later balance or later full snapshot is rejected', () => {
  for (const mutate of [data => { data.recovered_gift_points = 180; }, data => { data.restored = data.post_target; }, data => { data.post_target = data.target; }]) {
    const data = report(); mutate(data); assert.throws(() => checkPITREvidence(data), /cutoff/);
  }
});
test('missing archive, fencing or paused-target proof cannot pass', () => {
  for (const field of ['basebackup_before_restore_point', 'target_wal_archived', 'primary_stopped_before_recovery', 'recovered_readonly_at_target']) {
    const data = report(); data[field] = false; assert.throws(() => checkPITREvidence(data), /Missing PITR proof/);
  }
  const data = report(); data.archive_files = [];
  assert.throws(() => checkPITREvidence(data), /Missing actual WAL/);
});
test('archive filename traversal, malformed hashes and duplicate files fail', () => {
  for (const mutate of [data => { data.archive_files[0].file = '../original'; }, data => { data.archive_files[0].sha256 = 'not a hash'; }, data => { data.archive_files.push(data.archive_files[0]); }]) {
    const data = report(); mutate(data); assert.throws(() => checkPITREvidence(data), /Invalid archive/);
  }
});
test('lab results do not establish automatic switching or production acceptance', () => {
  const data = report(); data.production_disaster_recovery_accepted = true;
  assert.throws(() => checkPITREvidence(data), /cannot claim/);
});
test('current actual named restore point report passes full snapshot verification', () => {
  const data = JSON.parse(readFileSync(new URL('../../docs/performance/current-named-pitr-20261010.json', import.meta.url), 'utf8'));
  assert.equal(checkPITREvidence(data).tables, 120);
  assert.equal(data.target.row_count, 322);
});
