import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { checkRecoveryBaseline, checkPhysicalHistory } from '../../scripts/check-recovery-evidence.mjs';

const recovery = () => JSON.parse(readFileSync(new URL('../../docs/performance/current-recovery-baseline-20261010.json', import.meta.url), 'utf8'));
const history = () => JSON.parse(readFileSync(new URL('../../docs/performance/current-physical-history-20261010.json', import.meta.url), 'utf8'));
test('current physical recovery report verifies all table digests and the single baseline', () => {
  assert.deepEqual(checkRecoveryBaseline(recovery()), { controlled_recovery_passed: true, tables: 120, rows: 322, production_disaster_recovery_accepted: false });
});
test('table changes and count-only restoration cannot pass whole-snapshot checks', () => {
  const data = recovery();
  const name = Object.keys(data.restored.tables)[0];
  data.restored.tables[name].sha256 = 'a'.repeat(64);
  assert.throws(() => checkRecoveryBaseline(data), /digest/);
  const changed = recovery(); changed.evidence.restored_row_count++;
  assert.throws(() => checkRecoveryBaseline(changed), /counts/);
});
test('missing fencing and unverified production claims remain failures', () => {
  const data = recovery(); data.evidence.primary_stopped_before_promotion = false;
  assert.throws(() => checkRecoveryBaseline(data), /Missing recovery proof/);
  const unsafe = recovery(); unsafe.pitr_verified = true;
  assert.throws(() => checkRecoveryBaseline(unsafe), /cannot claim/);
});
test('current real HTTP history report covers both replicas and six gated routes', () => {
  assert.deepEqual(checkPhysicalHistory(history()), { controlled_history_routing_passed: true, replicas: 2, routes: 6 });
});
test('permission failures, unavailable selected replicas and original mutations cannot be hidden', () => {
  for (const field of ['http_permission_revocation_403', 'http_audit_failure_no_history_data', 'offline_replica_rejected', 'replica_query_timeout_error']) {
    const data = history(); data[field] = false;
    assert.throws(() => checkPhysicalHistory(data), /Missing history route proof/);
  }
  const changed = history(); changed.original_digest_after = 'a'.repeat(64);
  assert.throws(() => checkPhysicalHistory(changed), /Original database/);
});
