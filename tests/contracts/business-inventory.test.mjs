import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { commonSchemas, composeDocument } from '../../scripts/openapi-lib.mjs';
import { operations, schemas, sourceTables } from '../../scripts/openapi-business-inventory.mjs';

const backend = new URL('../../backend/', import.meta.url);
const go = process.env.LOTTERY_GO_BIN ?? 'go';
const expectedRoutes = ['GET /api/v1/admin/reconciliations/business-inventory'];
const doc = composeDocument([{ schemas, operations }], expectedRoutes.map(route => {
  const [method, path] = route.split(' ');
  return { method, path };
}));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:business-inventory', components: { schemas: { ...commonSchemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:business-inventory#/components/schemas/${name}` });

const expectedSources = [
  'bet_order_exceptions', 'bet_order_judgments', 'bet_orders',
  'commission_adjustment_heads', 'commission_adjustments', 'commission_allocations', 'commission_calculations',
  'commission_correction_balance_heads', 'commission_correction_cycle_holds', 'commission_correction_execution_steps',
  'commission_correction_execution_targets', 'commission_correction_executions', 'commission_correction_plan_steps',
  'commission_correction_plan_targets', 'commission_correction_plans', 'commission_cycle_steps',
  'commission_cycle_targets', 'commission_cycles', 'commission_earnings', 'commission_payment_targets',
  'commission_payments', 'commission_runs', 'draw_correction_failures', 'draw_correction_targets',
  'draw_corrections', 'period_cancellation_targets', 'period_cancellations', 'point_accounts', 'point_buckets',
  'point_ledger_entries', 'recharge_orders', 'reward_order_actions', 'reward_orders', 'settlement_calculations',
  'settlement_failures', 'settlement_jobs', 'settlement_targets', 'withdrawal_operation_receipts',
  'withdrawal_order_transitions', 'withdrawal_orders', 'withdrawal_turnover_cycles',
];
const coverage = sourceTables.map(source_table => ({
  source_table,
  source_row_count: '0',
  reference_count: '0',
  issue_count: '0',
}));
const issue = {
  source_table: 'point_accounts',
  source_id: '11111111-1111-4111-8111-111111111111',
  code: 'MISSING_PARENT_REFERENCE',
  reference_key: 'point_accounts_brand_id_brand_member_id_fkey',
  parent_table: 'brand_members',
};
const snapshot = (overrides = {}) => ({
  brand_id: '11111111-1111-4111-8111-111111111111',
  snapshot_at: '2026-10-09T12:00:00Z',
  schema_version: 1,
  source_row_count: '0',
  reference_count: '0',
  issue_count: '0',
  issues_truncated: false,
  consistent: true,
  fingerprint: 'a'.repeat(64),
  coverage,
  issues: [],
  ...overrides,
});

test('business inventory exposes one explicit, read-only admin contract', () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`), expectedRoutes);
  assert.equal(Object.keys(doc.paths).length, 1);
  const source = operations[0];
  const operation = doc.paths[source.path].get;
  assert.equal(source.auth, 'admin');
  assert.equal(source.tag, 'finance');
  assert.equal(source.brandHeader, true);
  assert.deepEqual(source.permissions, ['wallet.view.brand', 'wallet.view.platform']);
  assert.deepEqual(source.additionalErrorStatuses, [413]);
  assert.ok(operation.responses['413'], 'complete-source cap errors must be documented');
  assert.equal(source.requestBody, undefined);
  assert.equal(operation.requestBody, undefined);
  assert.deepEqual(operation.parameters.filter(parameter => parameter.in === 'query'), []);
  assert.deepEqual(operation.parameters.find(parameter => parameter.name === 'X-Brand-ID').schema, { $ref: '#/components/schemas/UUID' });
  assert.equal(operation.parameters.find(parameter => parameter.name === 'X-Brand-ID').required, true);
  assert.deepEqual(operation['x-permissions'], ['wallet.view.brand', 'wallet.view.platform']);
  assert.equal(operation.security.length, 2);
  assert.deepEqual(Object.keys(doc.paths[source.path]), ['get']);
  assert.match(source.description, /Accepts no query string or body/);
  assert.match(source.description, /no repair, wallet mutation, or financial authorization/);
});

test('inventory schemas are closed, require the documented fields, and freeze exact source coverage', () => {
  const snapshotSchema = schemas.FinanceBusinessInventorySnapshot;
  assert.deepEqual(Object.keys(snapshotSchema.properties), [
    'brand_id', 'snapshot_at', 'schema_version', 'source_row_count', 'reference_count', 'issue_count',
    'issues_truncated', 'consistent', 'fingerprint', 'coverage', 'issues',
  ]);
  assert.deepEqual(snapshotSchema.required, Object.keys(snapshotSchema.properties));
  assert.equal(snapshotSchema.additionalProperties, false);
  assert.equal(schemas.FinanceBusinessInventoryIssue.additionalProperties, false);
  assert.deepEqual(schemas.FinanceBusinessInventoryIssue.required, [
    'source_table', 'source_id', 'code', 'reference_key', 'parent_table',
  ]);
  assert.deepEqual(schemas.FinanceBusinessInventoryIssue.properties.code.enum, [
    'MISSING_PARENT_REFERENCE', 'MISSING_REQUIRED_LEDGER_REFERENCE',
  ]);
  assert.deepEqual(sourceTables, expectedSources);
  const coverageSchema = schemas.FinanceBusinessInventoryCoverage;
  assert.equal(coverageSchema.minItems, 41);
  assert.equal(coverageSchema.maxItems, 41);
  assert.equal(coverageSchema.prefixItems.length, 41);
  assert.equal(coverageSchema.items, false);
  assert.deepEqual(coverageSchema.prefixItems.map(row => row.properties.source_table.const), expectedSources);
  for (const row of coverageSchema.prefixItems) {
    assert.equal(row.additionalProperties, false);
    assert.deepEqual(Object.keys(row.properties), ['source_table', 'source_row_count', 'reference_count', 'issue_count']);
  }
  assert.equal(snapshotSchema.properties.brand_id.$ref, '#/components/schemas/UUID');
  assert.equal(snapshotSchema.properties.snapshot_at.format, 'date-time');
  assert.equal(snapshotSchema.properties.schema_version.const, 1);
  assert.equal(snapshotSchema.properties.source_row_count.pattern, '^(0|[1-9][0-9]{0,4}|100000)$');
  assert.equal(snapshotSchema.properties.fingerprint.pattern, '^[a-f0-9]{64}$');
  assert.equal(snapshotSchema.properties.issues.maxItems, 100);

  const checkSnapshot = validate('FinanceBusinessInventorySnapshot');
  assert.ok(checkSnapshot(snapshot()), JSON.stringify(checkSnapshot.errors));
  for (const invalid of [
    { ...snapshot(), extra: true },
    { ...snapshot(), account_id: 'private' },
    { ...snapshot(), wallet_account_id: 'private' },
    { ...snapshot(), schema_version: '1' },
    { ...snapshot(), schema_version: 2 },
    { ...snapshot(), brand_id: 'not-a-uuid' },
    { ...snapshot(), snapshot_at: 'yesterday' },
    { ...snapshot(), fingerprint: 'A'.repeat(64) },
  ]) assert.ok(!checkSnapshot(invalid), JSON.stringify(invalid));

  const checkIssue = validate('FinanceBusinessInventoryIssue');
  assert.ok(checkIssue(issue), JSON.stringify(checkIssue.errors));
  for (const invalid of [
    { ...issue, unexpected: 'private' },
    { ...issue, source_table: 'unknown_table' },
    { ...issue, source_id: 'non-ascii-é' },
    { ...issue, source_id: 'bad key' },
    { ...issue, source_id: 'x'.repeat(501) },
    { ...issue, code: 'UNKNOWN' },
    { ...issue, reference_key: 'Upper_case' },
    { ...issue, reference_key: `a${'b'.repeat(63)}` },
    { ...issue, parent_table: 'BrandMembers' },
  ]) assert.ok(!checkIssue(invalid), JSON.stringify(invalid));
});

test('inventory count syntax, source cap, issue cap, and representable consistency rules hold', () => {
  const check = validate('FinanceBusinessInventorySnapshot');
  const badCounts = [1, -1, 1.5, '01', '+1', ' 1', '1e3', ''];
  for (const value of badCounts) {
    assert.ok(!check(snapshot({ source_row_count: value })), `accepted snapshot count ${JSON.stringify(value)}`);
    assert.ok(!check(snapshot({ coverage: coverage.map((row, i) => i === 0 ? { ...row, reference_count: value } : row) })), `accepted coverage count ${JSON.stringify(value)}`);
  }
  assert.ok(!check(snapshot({ source_row_count: '100001' })), 'source row cap is 100000');
  assert.ok(!check(snapshot({ reference_count: '0'.repeat(201) })), 'counts are bounded to the backend parser length');
  assert.ok(!check(snapshot({ coverage: [...coverage.slice(1), coverage[0]] })), 'coverage order is fixed');
  assert.ok(!check(snapshot({ coverage: coverage.slice(1) })), 'all 41 source tables are required');
  assert.ok(!check(snapshot({ coverage: [...coverage, coverage[0]] })), 'coverage cannot contain extra tables');

  const oneIssueCoverage = coverage.map((row, i) => i === 27 ? { ...row, source_row_count: '1', reference_count: '1', issue_count: '1' } : row);
  assert.ok(check(snapshot({ source_row_count: '1', reference_count: '1', issue_count: '1', consistent: false, coverage: oneIssueCoverage, issues: [issue] })), JSON.stringify(check.errors));
  assert.ok(!check(snapshot({ consistent: false })), 'a clean inventory is consistent');
  assert.ok(!check(snapshot({ issue_count: '1' })), 'reported issues require inconsistent status and a sample');
  assert.ok(!check(snapshot({ source_row_count: '1', reference_count: '1', issue_count: '1', consistent: false, issues_truncated: true, issues: [issue] })), 'truncation implies more than 100 total issues and a full 100-item sample');

  const hundredIssues = Array.from({ length: 100 }, (_, i) => ({ ...issue, source_id: `row-${String(i).padStart(3, '0')}` }));
  assert.ok(check(snapshot({ reference_count: '100', issue_count: '100', consistent: false, issues: hundredIssues })), JSON.stringify(check.errors));
  assert.ok(!check(snapshot({ reference_count: '101', issue_count: '101', consistent: false, issues: [...hundredIssues, issue] })), 'the returned issue list is capped at 100');
  assert.ok(!check(snapshot({ reference_count: '101', issue_count: '101', consistent: false, issues: hundredIssues })), 'more than 100 issues requires issues_truncated');
  assert.ok(check(snapshot({ reference_count: '101', issue_count: '101', issues_truncated: true, consistent: false, issues: hundredIssues })), JSON.stringify(check.errors));
  assert.ok(!check(snapshot({ reference_count: '101', issue_count: '101', issues_truncated: true, consistent: false, issues: hundredIssues.slice(1) })), 'truncated issue sample contains 100 rows');
});

test('real Go FinanceBusinessInventorySnapshot example conforms to the contract', () => {
  const run = spawnSync(go, ['run', '-buildvcs=false', './cmd/contract-examples'], {
    cwd: backend,
    encoding: 'utf8',
    env: { ...process.env, CGO_ENABLED: '0' },
    maxBuffer: 8 * 1024 * 1024,
  });
  assert.equal(run.status, 0, run.stderr || run.error?.message);
  const examples = JSON.parse(run.stdout);
  const value = examples.FinanceBusinessInventorySnapshot;
  assert.ok(value, 'contract-examples must serialize FinanceBusinessInventorySnapshot');
  const check = validate('FinanceBusinessInventorySnapshot');
  assert.ok(check(value), JSON.stringify(check.errors));
  assert.deepEqual(Object.keys(value).sort(), Object.keys(schemas.FinanceBusinessInventorySnapshot.properties).sort());
  assert.deepEqual(value.coverage.map(row => row.source_table), expectedSources);
  assert.ok(value.coverage.every(row => Object.keys(row).length === 4));
  assert.ok(value.issues.every(row => Object.keys(row).length === 5));
  assert.ok(!check({ ...value, private_wallet_account_id: 'must-not-serialize' }));
});
