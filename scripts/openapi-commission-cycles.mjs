const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$' };
const version = { type: 'integer', minimum: 1, maximum: 9007199254740991 };
const state = { type: 'string', enum: ['enumerating', 'waiting', 'calculating', 'summarizing', 'ready', 'failed'] };
const page = [
  { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
const pageFields = items => ({ items: { type: 'array', items: ref(items), maxItems: 100 }, total_count: count, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } });
export const schemas = {
  CommissionCycle: {
    ...obj({ id: ref('UUID'), brand_id: ref('UUID'), window_from: ref('DateTime'), window_to: ref('DateTime'), anchor_order_id: ref('UUID'), calendar: ref('CommissionCalendar'), state, version, target_count: count, scan_complete: { type: 'boolean' }, current_run_id: nullable(ref('UUID')), current_generation: nullable(ref('PositiveInt64String')), evidence_epoch: nullable(ref('NonnegativeInt64String')), calculated_count: count, earning_count: count, total_points: count, created_by: nullable(ref('UUID')), creation_actor_type: { type: 'string', enum: ['admin', 'system'] }, reason: ref('Reason'), created_at: ref('DateTime'), updated_at: ref('DateTime'), last_error_code: nullable({ type: 'string' }), creation_audit_log_id: ref('UUID') }),
    description: 'Primary financial calculation projection, not paid points. ready means all current-generation calculations and rounded per-agent earnings are complete; no commission ledger credit, approval or payout exists yet. Counts and totals are exact decimal strings. The complete immutable bet manifest uses the UTC interval [window_from,window_to).',
  },
  CommissionCyclePage: obj({ brand_id: ref('UUID'), ...pageFields('CommissionCycle') }),
  CommissionCycleCreate: obj({ anchor_order_id: ref('UUID'), reason: ref('Reason') }),
  CommissionCycleRetry: obj({ version, reason: ref('Reason') }),
  CommissionExactAmount: obj({ numerator: { ...count, maxLength: 100 }, denominator: { type: 'string', pattern: '^[1-9][0-9]*$', maxLength: 7 } }),
  CommissionCycleEarning: obj({ id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), run_id: ref('UUID'), agent_id: ref('UUID'), member_id: ref('UUID'), exact_amount: ref('CommissionExactAmount'), points: ref('NonnegativeInt64String'), created_at: ref('DateTime') }),
  CommissionEarningPage: obj({ brand_id: ref('UUID'), cycle_id: ref('UUID'), ...pageFields('CommissionCycleEarning') }),
  CommissionRun: obj({ id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), generation: ref('PositiveInt64String'), evidence_epoch: ref('NonnegativeInt64String'), state: { type: 'string', enum: ['calculating', 'summarizing', 'ready', 'abandoned'] }, calculated_count: count, earning_count: count, total_points: count, created_at: ref('DateTime') }),
  CommissionRunPage: obj({ brand_id: ref('UUID'), cycle_id: ref('UUID'), ...pageFields('CommissionRun') }),
  CommissionCalculation: obj({ id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), run_id: ref('UUID'), order_id: ref('UUID'), member_id: ref('UUID'), reason: { type: 'string', enum: ['eligible', 'policy_disabled', 'unattributed', 'other_cycle', 'cancelled', 'abnormal'] }, status: { type: 'string', enum: ['won', 'lost', 'abnormal', 'bet_cancelled', 'judged_cancelled'] }, stake_points: ref('PositiveInt64String'), prize_points: ref('NonnegativeInt64String'), base_points: ref('NonnegativeInt64String'), job_id: nullable(ref('UUID')), calculation_id: nullable(ref('UUID')), generation: nullable(ref('PositiveInt64String')), audit_log_id: ref('UUID'), created_at: ref('DateTime') }),
  CommissionCalculationPage: obj({ brand_id: ref('UUID'), cycle_id: ref('UUID'), run_id: ref('UUID'), ...pageFields('CommissionCalculation') }),
};
schemas.CommissionCycle.required = schemas.CommissionCycle.required.filter(name => name !== 'creation_actor_type');
schemas.CommissionCycle.properties.creation_actor_type.description = 'Present in new and read DTOs; optional only for historical cached receipts created before migration 0049, where absence means admin. system identifies discovery-created cycles and is present exactly when created_by is null.';
schemas.CommissionCycle.properties.created_by.description = 'Null exactly when creation_actor_type is system.';
schemas.CommissionCycle.properties.evidence_current = { type: 'boolean', description: 'False if the current run is absent or a related final settlement changed. ready plus true is still not approval or payout authorization.' };
schemas.CommissionCycle.required.push('evidence_current');
const admin = (method, suffix, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin/commission-cycles${suffix}`, operationId, summary, tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: ['commission.view.brand', 'commission.view.platform'],
  description: 'Primary-only, audited, current-session brand/platform view. Unknown, duplicate or empty query parameters are rejected. Private financial snapshots, agent paths, wallet account IDs and worker cursors are omitted.', ...extra,
});
export const operations = [
  admin('GET', '', 'listCommissionCycles', 'List actual commission cycle calculations', ref('CommissionCyclePage'), { parameters: page }),
  admin('POST', '', 'createCommissionCycle', 'Create a closed-window commission cycle', ref('CommissionCycle'), { permissions: ['commission.run.brand'], requestBody: ref('CommissionCycleCreate'), idempotency: true, successStatus: 201,
    description: 'Requires commission view plus exact commission.run.brand and denies super-admin writes. Resolves the enabled immutable financial policy of a real anchor bet, not current policy; UTC window must already have closed. A short financial-policy admission barrier freezes the manifest; backdated inserts into a registered window are rejected. A new key cannot create a second cycle for the same brand/from/to. Worker pages wait for all orders to become final, persist exact differential allocations, sum per agent and round half-up once. Cached receipt rechecks authorization and returns the original enumerating receipt, not later worker state; same key with changed validated raw body conflicts. No wallet credit or payout is performed.',
  }),
  admin('GET', '/{id}', 'getCommissionCycle', 'Read a current commission cycle', ref('CommissionCycle')),
  admin('GET', '/{id}/earnings', 'listCommissionCycleEarnings', 'Read current-run calculated earnings', ref('CommissionEarningPage'), { parameters: page, description: 'Calculated amounts, not paid balances. Uses the current run only; old runs remain available from the run history. Primary audited view, strict pagination and no private rule snapshots.' }),
  admin('POST', '/{id}/retry', 'retryCommissionCycle', 'Retry a failed commission calculation', ref('CommissionCycle'), { permissions: ['commission.retry.brand'], requestBody: ref('CommissionCycleRetry'), idempotency: true,
    description: 'Requires view plus commission.retry.brand; super-admin denied. Only failed cycles at the supplied version can retry. Preserves prior calculations and failures, abandons the previous run and resumes enumeration or waiting. Current session/permission and brand status are checked before receipt replay. Transient period lock contention is retryable, does not cache a terminal failure, and requires the original key/body. No approval or payout is performed.',
  }),
  admin('GET', '/{id}/runs', 'listCommissionCycleRuns', 'Read preserved calculation generations', ref('CommissionRunPage'), { parameters: page }),
  admin('GET', '/{id}/runs/{runID}/calculations', 'listCommissionRunCalculations', 'Read sanitized calculations for an explicit run', ref('CommissionCalculationPage'), { parameters: page }),
];
