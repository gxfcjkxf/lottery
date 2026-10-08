import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-commission-corrections.mjs';
import { composeDocument, documentedRoutes } from '../../scripts/openapi-lib.mjs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addFormat('nonnegative-int64-string', { type: 'string', validate: value => /^(0|[1-9][0-9]*)$/.test(value) && BigInt(value) <= 9223372036854775807n });
ajv.addSchema({ $id: 'urn:lottery:commission-correction-contract', components: { schemas: { ...doc.components.schemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:commission-correction-contract#/components/schemas/${name}` });

test('commission correction contract declares exactly the twelve canonical admin operations', () => {
  assert.deepEqual(operations.map(({ method, path }) => `${method} ${path}`).sort(), [
    'GET /api/v1/admin/commission-correction-policy',
    'PUT /api/v1/admin/commission-correction-policy',
    'GET /api/v1/admin/commission-correction-plans',
    'GET /api/v1/admin/commission-correction-plans/{id}',
    'GET /api/v1/admin/commission-correction-plans/{id}/targets',
    'POST /api/v1/admin/commission-correction-plans/{id}/retry',
    'GET /api/v1/admin/commission-correction-executions',
    'GET /api/v1/admin/commission-correction-executions/{id}',
    'GET /api/v1/admin/commission-correction-executions/{id}/targets',
    'POST /api/v1/admin/commission-correction-executions/{id}/approve',
    'POST /api/v1/admin/commission-correction-executions/{id}/continue',
    'POST /api/v1/admin/commission-correction-executions/{id}/retry',
  ].sort());
  for (const operation of operations) {
    assert.equal(operation.auth, 'admin');
    assert.equal(operation.brandHeader, true);
    assert.ok(!operation.path.includes('/b/{brandCode}/'));
    if (operation.method !== 'GET') {
      assert.equal(operation.idempotency, true);
      assert.equal(operation.successStatus, undefined, 'mutations use the canonical 200 receipt response');
      assert.deepEqual(operation.parameters.map(({ name, in: location, required }) => [name, location, required]), [
        ['X-Commission-Correction-Actor-ID', 'header', true],
      ]);
      assert.equal(operation.additionalErrorStatuses, undefined);
      assert.match(operation.description, /409 conflict retains the original intent/);
      assert.match(operation.description, /SQLSTATE 55P03 busy maps to 503 and is not cached/);
      assert.match(operation.description, /checked again after business and audit writes, immediately before commit/);
      assert.match(operation.description, /above JavaScript's maximum safe integer fails closed input validation with 400/);
      assert.match(operation.description, /equal to the maximum safe integer passes input decoding, but a domain version increment overflow returns 409/);
      const assembled = composeDocument([{ schemas, operations: [operation] }], [{ method: operation.method, path: operation.path }]);
      assert.ok(assembled.paths[operation.path][operation.method.toLowerCase()].responses['200']);
      assert.ok(assembled.paths[operation.path][operation.method.toLowerCase()].responses['409']);
      assert.ok(assembled.paths[operation.path][operation.method.toLowerCase()].responses['503']);
    }
  }
  assert.deepEqual(operations.find(op => op.operationId === 'updateCommissionCorrectionPolicy').permissions,
    ['commission.view.brand', 'commission.view.platform', 'commission_correction_policy.write.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'retryCommissionCorrectionPlan').permissions,
    ['commission.view.brand', 'commission.view.platform', 'commission_correction.retry.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'approveCommissionCorrectionExecution').permissions,
    ['commission.view.brand', 'commission.view.platform', 'commission_correction.approve.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'continueCommissionCorrectionExecution').permissions,
    ['commission.view.brand', 'commission.view.platform', 'commission_correction.continue.brand']);
  assert.deepEqual(operations.find(op => op.operationId === 'retryCommissionCorrectionExecution').permissions,
    ['commission.view.brand', 'commission.view.platform', 'commission_correction.execute_retry.brand']);
});

test('correction schemas are closed and the exact request and numeric contracts are explicit', () => {
  for (const [name, schema] of Object.entries(schemas)) {
    assert.equal(schema.additionalProperties, false, `${name} must be closed`);
    assert.doesNotThrow(() => validate(name), `${name} compiles`);
    assert.ok(!Object.keys(schema.properties ?? {}).some(field => /signature|scope/i.test(field)), `${name} must not expose signature or scope fields`);
  }
  assert.deepEqual(schemas.CommissionCorrectionExecutionPolicyInput.required, ['version', 'enabled', 'reason']);
  assert.deepEqual(schemas.CommissionCorrectionAction.required, ['version', 'reason']);
  for (const input of [schemas.CommissionCorrectionExecutionPolicyInput, schemas.CommissionCorrectionAction]) {
    assert.equal(input.properties.version.maximum, Number.MAX_SAFE_INTEGER);
    assert.match(input.properties.reason.description, /UTF-8 bytes/);
    const check = validate(input === schemas.CommissionCorrectionAction ? 'CommissionCorrectionAction' : 'CommissionCorrectionExecutionPolicyInput');
    const maxVersion = input === schemas.CommissionCorrectionAction
      ? { version: Number.MAX_SAFE_INTEGER, reason: 'maximum safe version' }
      : { version: Number.MAX_SAFE_INTEGER, enabled: false, reason: 'maximum safe version' };
    assert.ok(check(maxVersion), 'MAX_SAFE_INTEGER passes input decoding');
    const tooLargeVersion = { ...maxVersion, version: Number.MAX_SAFE_INTEGER + 1 };
    assert.ok(!check(tooLargeVersion), 'versions above MAX_SAFE_INTEGER fail closed input validation');
    for (const invalid of ['', '   ', ' leading', 'trailing ', '\nline', 'line\n', 'line\tbreak', 'line\u0085break']) {
      const value = input === schemas.CommissionCorrectionAction ? { version: 1, reason: invalid } : { version: 1, enabled: false, reason: invalid };
      assert.ok(!check(value), `accepted invalid reason ${JSON.stringify(invalid)}`);
    }
  }
  for (const operation of operations.filter(op => op.method === 'GET')) {
    assert.equal(Object.hasOwn(operation, 'requestBody'), false);
    assert.match(operation.description, /Every GET rejects a body and ForceQuery/);
    const paged = operation.path === '/api/v1/admin/commission-correction-plans' || operation.path === '/api/v1/admin/commission-correction-executions' || operation.path.endsWith('/targets');
    assert.deepEqual((operation.parameters ?? []).map(parameter => parameter.name), paged ? ['limit', 'offset'] : []);
  }
  assert.deepEqual(schemas.CommissionCorrectionPlan.properties.evidence_epoch, schemas.CommissionCorrectionExecution.properties.evidence_epoch);
  assert.match(schemas.CommissionCorrectionPlan.properties.evidence_epoch.description, /register the custom format nonnegative-int64-string/);
  assert.match(schemas.CommissionCorrectionPlan.properties.evidence_epoch.description, /generic OpenAPI readers apply only canonical syntax and length/);
  assert.equal(schemas.CommissionCorrectionExecutionPolicy.properties.audit_log_id.anyOf[1].const, '');
  assert.equal(schemas.CommissionCorrectionPlanTarget.properties.adjustment_version.anyOf[0].minimum, 1);
  assert.equal(schemas.CommissionCorrectionPlanTarget.properties.financial_version.anyOf[0].minimum, 1);
  assert.equal(schemas.CommissionCorrectionExecutionTarget.properties.financial_version.anyOf[0].minimum, 1);
  assert.deepEqual(schemas.CommissionCorrectionExecution.properties.approval_actor_type.anyOf[0].enum, ['admin', 'system']);
  assert.equal(schemas.CommissionCorrectionPlanTarget.properties.delta_points.$ref, '#/components/schemas/Int64String');
  assert.equal(schemas.CommissionCorrectionExecutionTarget.properties.delta_points.$ref, '#/components/schemas/Int64String');
  for (const [name, fields] of Object.entries({
    CommissionCorrectionPlan: ['before_points', 'calculated_points', 'credit_points', 'debit_points'],
    CommissionCorrectionExecution: ['credit_points', 'debit_points', 'applied_credit_points', 'applied_debit_points'],
  })) {
    for (const field of fields) {
      const unsigned = schemas[name].properties[field].anyOf?.[0] ?? schemas[name].properties[field];
      assert.equal(unsigned.type, 'string');
      assert.equal(unsigned.pattern, '^(0|[1-9][0-9]*)$');
    }
    const signedNet = schemas[name].properties.net_points.anyOf?.[0] ?? schemas[name].properties.net_points;
    assert.equal(signedNet.pattern, '^(0|-?[1-9][0-9]*)$');
  }
  assert.equal(schemas.CommissionCorrectionExecution.properties.last_error_code.anyOf[1].type, 'null');
  assert.match(schemas.CommissionCorrectionPlan.description, /does not approve or move funds/);
  assert.match(schemas.CommissionCorrectionExecution.description, /mixed payout mode requires fresh approval/);
  assert.match(schemas.CommissionCorrectionExecution.description, /hold inherited by later runs unless explicitly continued/);
  assert.match(operations.find(op => op.operationId === 'continueCommissionCorrectionExecution').description, /new draws inherit the hold/);
  assert.match(operations.find(op => op.operationId === 'continueCommissionCorrectionExecution').description, /subsequent runs can execute normally/);
  assert.match(schemas.CommissionCorrectionPlan.description, /RFC3339Nano/);
  const checkEpoch = ajv.compile(schemas.CommissionCorrectionPlan.properties.evidence_epoch);
  assert.ok(checkEpoch('9223372036854775807'));
  assert.ok(!checkEpoch('9223372036854775808'));
  assert.ok(!checkEpoch('-1'));
  const checkNullableVersion = ajv.compile(schemas.CommissionCorrectionPlanTarget.properties.financial_version);
  assert.ok(checkNullableVersion(null));
  assert.ok(checkNullableVersion(1));
  assert.ok(!checkNullableVersion(0));
  assert.ok(!ajv.compile(schemas.CommissionCorrectionExecution.properties.approval_actor_type)('other'));
  assert.ok(!Object.keys(schemas.CommissionCorrectionPlanTarget.properties).some(name => /account|wallet|rule|bucket/i.test(name)));
  const assembled = composeDocument([{ schemas, operations }], operations.map(({ method, path }) => ({ method, path })));
  assert.equal(documentedRoutes(assembled).length, 12);
  assert.ok(!Object.keys(assembled.paths).some(path => path.includes('/api/v1/b/')));
});

test('actual Go contract examples validate all correction DTO schemas and preserve exact aggregate behavior', () => {
  const result = spawnSync(process.env.LOTTERY_GO_BIN ?? 'go', [
    'run', '-buildvcs=false', './cmd/contract-examples',
  ], { cwd: new URL('../../backend/', import.meta.url), encoding: 'utf8', env: { ...process.env, CGO_ENABLED: '0' } });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  const examples = JSON.parse(result.stdout);
  const schemasByKey = {
    correction_policy: 'CommissionCorrectionExecutionPolicy',
    correction_policy_updated: 'CommissionCorrectionExecutionPolicy',
    correction_plan: 'CommissionCorrectionPlan',
    correction_plan_target: 'CommissionCorrectionPlanTarget',
    correction_execution: 'CommissionCorrectionExecution',
    correction_execution_target: 'CommissionCorrectionExecutionTarget',
  };
  for (const [key, schema] of Object.entries(schemasByKey)) {
    assert.ok(Object.hasOwn(examples, key), `contract-examples missing ${key}`);
    const check = validate(schema);
    assert.ok(check(examples[key]), `${key}: ${JSON.stringify(check.errors)}`);
  }
  assert.equal(examples.correction_policy.version, 1);
  assert.equal(examples.correction_policy.enabled, false);
  assert.equal(examples.correction_policy.audit_log_id, '');
  assert.equal(examples.correction_policy_updated.version, 2);
  assert.equal(examples.correction_policy_updated.enabled, true);
  assert.equal(examples.correction_plan.credit_points, '100');
  assert.equal(examples.correction_plan.debit_points, '100');
  assert.equal(examples.correction_plan.net_points, '0');
  assert.equal(examples.correction_plan.evidence_epoch, '9223372036854775807');
  assert.equal(examples.correction_plan_target.points_before, '0');
  assert.equal(examples.correction_plan_target.points_after, '100');
  assert.notEqual(examples.correction_plan_target.earning_id, null);
  assert.equal(examples.correction_plan_target.original_target_id, null);
  assert.equal(examples.correction_plan_target.previous_correction_target_id, null);
  assert.equal(examples.correction_execution.state, 'awaiting_approval');
  assert.equal(examples.correction_execution.payout_mode, 'mixed');
  assert.equal(examples.correction_execution.target_count, '2');
  for (const field of ['approved_by', 'approval_actor_type', 'approval_audit_log_id']) assert.equal(examples.correction_execution[field], null);
  assert.equal(examples.correction_execution_target.state, 'applied');
  assert.equal(examples.correction_execution_target.points_before, '100');
  assert.equal(examples.correction_execution_target.points_after, '0');
  assert.equal(examples.correction_execution_target.delta_points, '-100');
  assert.notEqual(examples.correction_execution_target.ledger_entry_id, null);
  assert.notEqual(examples.correction_execution_target.audit_log_id, null);
  assert.equal(examples.correction_execution_target.financial_version, 1);
  for (const key of Object.keys(schemasByKey)) {
    const dto = examples[key];
    for (const field of ['created_at', 'updated_at', 'applied_at']) {
      if (typeof dto[field] === 'string') assert.match(dto[field], /\.123456789Z$/i, `${key}.${field} must preserve nanoseconds`);
    }
    if (typeof dto.updated_at === 'string') assert.match(dto.updated_at, /\.123456789Z$/i);
  }
  assert.ok(validate('CommissionCorrectionPlanPage')({ brand_id: examples.correction_plan.brand_id, items: [examples.correction_plan], total_count: '1', limit: 20, offset: 0 }));
  assert.ok(validate('CommissionCorrectionExecutionTargetPage')({ brand_id: examples.correction_execution_target.brand_id, execution_id: examples.correction_execution_target.execution_id, items: [examples.correction_execution_target], total_count: '1', limit: 20, offset: 0 }));
});
