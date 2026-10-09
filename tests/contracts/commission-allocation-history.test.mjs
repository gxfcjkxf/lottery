import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { operations, schemas } from '../../scripts/openapi-commission-cycles.mjs';

const doc = JSON.parse(readFileSync(new URL('../../docs/openapi.json', import.meta.url), 'utf8'));
const ajv = new Ajv2020({ strict: false, allErrors: true });
addFormats(ajv);
ajv.addSchema({ $id: 'urn:lottery:commission-allocation-history', components: { schemas: { ...doc.components.schemas, ...schemas } } });
const validate = name => ajv.compile({ $ref: `urn:lottery:commission-allocation-history#/components/schemas/${name}` });

const brand = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const cycle = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const run = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc';
const allocationID = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';
const order = 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee';
const agent = 'ffffffff-ffff-4fff-8fff-ffffffffffff';
const member = '11111111-1111-4111-8111-111111111111';
const timestamp = '2026-10-09T00:00:00Z';
const bigintGCD = (left, right) => {
  let a = left < 0n ? -left : left;
  let b = right < 0n ? -right : right;
  while (b !== 0n) [a, b] = [b, a % b];
  return a;
};
const differentialFraction = (base, differenceMicros) => {
  const numerator = BigInt(base) * BigInt(differenceMicros);
  const denominator = 1_000_000n;
  const divisor = bigintGCD(numerator, denominator);
  return { numerator: (numerator / divisor).toString(), denominator: (denominator / divisor).toString() };
};

test('historical earnings and allocations expose exactly two audited readonly routes', () => {
  const history = operations.filter(operation => /\/runs\/\{runID\}\/(?:earnings|allocations)$/.test(operation.path));
  assert.deepEqual(history.map(({ method, path }) => `${method} ${path}`).sort(), [
    'GET /api/v1/admin/commission-cycles/{id}/runs/{runID}/allocations',
    'GET /api/v1/admin/commission-cycles/{id}/runs/{runID}/earnings',
  ]);
  for (const operation of history) {
    assert.equal(operation.auth, 'admin');
    assert.equal(operation.brandHeader, true);
    assert.deepEqual(operation.permissions, ['commission.view.brand', 'commission.view.platform']);
    assert.equal(operation.idempotency, undefined);
    assert.equal(operation.requestBody, undefined);
    assert.equal(operation.routingHeaders, undefined);
    assert.doesNotMatch(operation.path, /payout|payment|approve|balance/i);
    assert.match(operation.description, /ForceQuery \(including a trailing empty \?\)/);
    assert.match(operation.description, /GET body, nonzero Content-Length or any Transfer-Encoding return 400/i);
    assert.match(operation.description, /does not store the raw URL or request body/i);
  }

  const earnings = history.find(operation => operation.path.endsWith('/earnings'));
  assert.equal(earnings.data.$ref, '#/components/schemas/CommissionRunEarningPage');
  assert.deepEqual(earnings.parameters.map(({ name, in: location, required }) => [name, location, required]), [
    ['limit', 'query', false], ['offset', 'query', false],
  ]);
  assert.match(earnings.description, /explicitly selected historical or current run; never substitutes the cycle current run/i);
  assert.match(earnings.description, /rounded per agent for the whole cycle/i);
  assert.match(earnings.description, /commission\.cycle\.run_earnings audit stores only validated cycle_id, run_id, limit and offset/i);

  const allocations = history.find(operation => operation.path.endsWith('/allocations'));
  assert.equal(allocations.data.$ref, '#/components/schemas/CommissionAllocationPage');
  assert.deepEqual(allocations.parameters.map(({ name, in: location, required }) => [name, location, required]), [
    ['limit', 'query', false], ['offset', 'query', false], ['agent_id', 'query', false], ['order_id', 'query', false],
  ]);
  assert.deepEqual(allocations.permissions, earnings.permissions);
  assert.match(allocations.description, /without consulting today’s node policy/i);
  assert.match(allocations.description, /Unknown, foreign-cycle\/run and optional-filter resources return 404; a matching empty page returns 200/i);
  assert.match(allocations.description, /unknown, duplicate, empty or noncanonical query parameters.*return 400/i);
  assert.match(allocations.description, /not earnings, payouts or balances/i);
  assert.match(allocations.description, /commission\.cycle\.run_allocations audit stores only validated cycle_id, run_id, limit, offset, agent_id and order_id/i);
});

test('allocation history schemas are closed, exact and contain only captured public evidence', () => {
  assert.deepEqual(schemas.CommissionRunEarningPage.required, ['brand_id', 'cycle_id', 'run_id', 'items', 'total_count', 'limit', 'offset']);
  assert.equal(schemas.CommissionRunEarningPage.properties.items.items.$ref, '#/components/schemas/CommissionCycleEarning');
  assert.deepEqual(schemas.CommissionAllocation.required, [
    'brand_id', 'cycle_id', 'run_id', 'calculation_id', 'order_id', 'agent_id', 'member_id', 'bettor_member_id',
    'base_points', 'mode', 'agent_ratio', 'downstream_ratio', 'difference_ratio', 'exact_amount', 'created_at',
  ]);
  assert.deepEqual(schemas.CommissionAllocation.properties.mode.enum, ['loss', 'turnover']);
  assert.equal(schemas.CommissionAllocation.properties.base_points.$ref, '#/components/schemas/NonnegativeInt64String');
  assert.equal(schemas.CommissionAllocation.properties.exact_amount.$ref, '#/components/schemas/CommissionExactAmount');
  assert.deepEqual(schemas.CommissionAllocationPage.required, ['brand_id', 'cycle_id', 'run_id', 'agent_id', 'order_id', 'items', 'total_count', 'limit', 'offset']);
  assert.equal(schemas.CommissionAllocationPage.properties.agent_id.anyOf[1].type, 'null');
  assert.equal(schemas.CommissionAllocationPage.properties.order_id.anyOf[1].type, 'null');

  for (const [name, schema] of Object.entries(schemas)) assert.equal(schema.additionalProperties, false, `${name} must be closed`);
  for (const name of ['CommissionAllocation', 'CommissionAllocationPage', 'CommissionRunEarningPage']) {
    assert.ok(!Object.keys(schemas[name].properties).some(field => /snapshot|path|account|wallet|cursor/i.test(field)), `${name} must omit private witness data`);
  }
  for (const field of ['agent_ratio', 'downstream_ratio', 'difference_ratio']) {
    assert.match(schemas.CommissionAllocation.properties[field].pattern, /1/);
    assert.equal(schemas.CommissionAllocation.properties[field].type, 'string');
  }
});

test('allocation and page examples preserve large point values and reject rounding, rates outside bounds, and private fields', () => {
  const basePoints = '9007199254740993';
  const expectedExact = differentialFraction(basePoints, 3455);
  const allocation = {
    brand_id: brand, cycle_id: cycle, run_id: run, calculation_id: allocationID, order_id: order,
    agent_id: agent, member_id: member, bettor_member_id: '22222222-2222-4222-8222-222222222222',
    base_points: '9007199254740993', mode: 'turnover', agent_ratio: '0.123456',
    downstream_ratio: '0.120001', difference_ratio: '0.003455',
    exact_amount: expectedExact, created_at: timestamp,
  };
  const checkAllocation = validate('CommissionAllocation');
  assert.ok(checkAllocation(allocation), JSON.stringify(checkAllocation.errors));
  for (const invalid of [
    { ...allocation, base_points: 9007199254740993 },
    { ...allocation, agent_ratio: '1.000001' },
    { ...allocation, downstream_ratio: '0.1234567' },
    { ...allocation, difference_ratio: 0.5 },
    { ...allocation, mode: 'current_policy' },
    { ...allocation, raw_snapshot: {} },
    { ...allocation, agent_path: [agent] },
    { ...allocation, account_id: allocationID },
  ]) assert.ok(!checkAllocation(invalid), JSON.stringify(checkAllocation.errors));

  const earning = {
    id: allocationID, brand_id: brand, cycle_id: cycle, run_id: run, agent_id: agent, member_id: member,
    exact_amount: { numerator: basePoints, denominator: '1' },
    points: '9007199254740993', created_at: timestamp,
  };
  const earningsPage = {
    brand_id: brand, cycle_id: cycle, run_id: run, items: [earning], total_count: '1', limit: 20, offset: 0,
  };
  const checkEarnings = validate('CommissionRunEarningPage');
  assert.ok(checkEarnings(earningsPage), JSON.stringify(checkEarnings.errors));
  assert.ok(!checkEarnings({ ...earningsPage, items: [{ ...earning, points: 9007199254740993 }] }));

  const allocationsPage = {
    brand_id: brand, cycle_id: cycle, run_id: run, agent_id: agent, order_id: order,
    items: [allocation], total_count: '1', limit: 20, offset: 0,
  };
  const checkAllocationsPage = validate('CommissionAllocationPage');
  assert.ok(checkAllocationsPage(allocationsPage), JSON.stringify(checkAllocationsPage.errors));
  assert.ok(checkAllocationsPage({ ...allocationsPage, agent_id: null, order_id: null, items: [] }));
  assert.ok(!checkAllocationsPage({ ...allocationsPage, items: [{ ...allocation, wallet_account_id: allocationID }] }));
});

test('large differential amount is reduced exactly with BigInt, without JavaScript Number arithmetic', () => {
  const exact = differentialFraction('9007199254740993', 3455);
  assert.deepEqual(exact, {
    numerator: (BigInt('9007199254740993') * 691n).toString(),
    denominator: '200000',
  });
  assert.equal(BigInt(exact.numerator) * 1_000_000n, BigInt('9007199254740993') * 3455n * BigInt(exact.denominator));
});

test('real commission allocation DTO examples match the schemas and exact BigInt arithmetic', () => {
  const result = spawnSync(process.env.LOTTERY_GO_BIN ?? 'go', ['run', './cmd/contract-examples'], {
    cwd: new URL('../../backend/', import.meta.url), encoding: 'utf8', maxBuffer: 16 * 1024 * 1024,
  });
  assert.equal(result.status, 0, result.stderr || result.error?.message);
  const examples = JSON.parse(result.stdout);
  const allocation = examples.CommissionAllocation;
  const allocationPage = examples.CommissionAllocationPage;
  const earningPage = examples.CommissionRunEarningPage;
  assert.ok(allocation && allocationPage && earningPage, 'Go examples must expose all three commission history keys');

  for (const [name, example] of [
    ['CommissionAllocation', allocation],
    ['CommissionAllocationPage', allocationPage],
    ['CommissionRunEarningPage', earningPage],
  ]) {
    const check = validate(name);
    assert.ok(check(example), `${name}: ${JSON.stringify(check.errors)}`);
    assert.deepEqual(Object.keys(example).sort(), Object.keys(schemas[name].properties).sort(), `${name} must serialize only public DTO fields`);
  }
  assert.equal(allocation.base_points, '9007199254740993');
  assert.equal(typeof allocation.base_points, 'string');
  assert.deepEqual(allocationPage.items, [allocation]);
  assert.equal(allocationPage.agent_id, null);
  assert.equal(allocationPage.order_id, null);
  assert.equal(earningPage.agent_id, undefined);
  assert.equal(earningPage.order_id, undefined);
  assert.deepEqual(earningPage.items, []);

  const base = BigInt(allocation.base_points);
  const ratioMicros = BigInt(allocation.agent_ratio.replace('.', '')) - BigInt(allocation.downstream_ratio.replace('.', ''));
  const actualNumerator = BigInt(allocation.exact_amount.numerator);
  const actualDenominator = BigInt(allocation.exact_amount.denominator);
  assert.equal(actualNumerator * 1_000_000n, base * ratioMicros * actualDenominator,
    'serialized exact_amount must equal base_points × (agent_ratio − downstream_ratio)');
  assert.equal(allocation.difference_ratio, '0.003455');

  assert.deepEqual(allocation.exact_amount, differentialFraction(allocation.base_points, 3455));
});
