const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = properties => ({ type: 'object', properties, required: Object.keys(properties), additionalProperties: false });
const count = {
  type: 'string',
  pattern: '^(0|[1-9][0-9]*)$',
  maxLength: 200,
  description: 'Canonical nonnegative decimal string. Never parse or aggregate using floating-point numbers.',
};
const sourceRowCount = {
  ...count,
  pattern: '^(0|[1-9][0-9]{0,4}|100000)$',
  maxLength: 6,
  description: 'Canonical nonnegative decimal string. The complete source row total is capped at 100000; the server enforces the aggregate using exact integer arithmetic.',
};

export const sourceTables = [
  'bet_order_exceptions',
  'bet_order_judgments',
  'bet_orders',
  'commission_adjustment_heads',
  'commission_adjustments',
  'commission_allocations',
  'commission_calculations',
  'commission_correction_balance_heads',
  'commission_correction_cycle_holds',
  'commission_correction_execution_steps',
  'commission_correction_execution_targets',
  'commission_correction_executions',
  'commission_correction_plan_steps',
  'commission_correction_plan_targets',
  'commission_correction_plans',
  'commission_cycle_steps',
  'commission_cycle_targets',
  'commission_cycles',
  'commission_earnings',
  'commission_payment_targets',
  'commission_payments',
  'commission_runs',
  'draw_correction_failures',
  'draw_correction_targets',
  'draw_corrections',
  'period_cancellation_targets',
  'period_cancellations',
  'point_accounts',
  'point_buckets',
  'point_ledger_entries',
  'recharge_orders',
  'reward_order_actions',
  'reward_orders',
  'settlement_calculations',
  'settlement_failures',
  'settlement_jobs',
  'settlement_targets',
  'withdrawal_operation_receipts',
  'withdrawal_order_transitions',
  'withdrawal_orders',
  'withdrawal_turnover_cycles',
];

const coverageRow = sourceTable => obj({
  source_table: { type: 'string', const: sourceTable },
  source_row_count: sourceRowCount,
  reference_count: count,
  issue_count: count,
});

export const schemas = {
  FinanceBusinessInventoryCoverage: {
    type: 'array',
    minItems: sourceTables.length,
    maxItems: sourceTables.length,
    prefixItems: sourceTables.map(coverageRow),
    items: false,
    description: 'Exactly one coverage row for each source table, in the fixed source_table order. The sum of source_row_count is at most 100000. The three row counts sum exactly to their corresponding snapshot totals; the service validates these decimal-string sums with exact integer arithmetic.',
  },
  FinanceBusinessInventoryIssue: {
    ...obj({
      source_table: { type: 'string', enum: sourceTables },
      source_id: { type: 'string', pattern: '^[A-Za-z0-9_.:/-]{1,500}$', maxLength: 500, description: 'Composite primary key projection, restricted to ASCII.' },
      code: { type: 'string', enum: ['MISSING_PARENT_REFERENCE', 'MISSING_REQUIRED_LEDGER_REFERENCE'] },
      reference_key: { type: 'string', pattern: '^[a-z][a-z0-9_]{0,62}$', maxLength: 63 },
      parent_table: { type: 'string', pattern: '^[a-z][a-z0-9_]{0,62}$', maxLength: 63 },
    }),
    description: 'One missing structural parent or required ledger pointer. Issues are ordered by source_table, source_id, reference_key, code, and parent_table. Parent brand or identity details and business row contents are not exposed.',
  },
  FinanceBusinessInventorySnapshot: {
    ...obj({
      brand_id: ref('UUID'),
      snapshot_at: { type: 'string', format: 'date-time' },
      schema_version: { type: 'integer', const: 1 },
      source_row_count: sourceRowCount,
      reference_count: count,
      issue_count: count,
      issues_truncated: { type: 'boolean' },
      consistent: { type: 'boolean', description: 'True exactly when issue_count is zero.' },
      fingerprint: { type: 'string', pattern: '^[a-f0-9]{64}$', minLength: 64, maxLength: 64, description: 'Lowercase SHA-256 hex digest bound to this brand, schema version, all source rows, and the complete issue set, including issues beyond the returned sample.' },
      coverage: ref('FinanceBusinessInventoryCoverage'),
      issues: { type: 'array', items: ref('FinanceBusinessInventoryIssue'), maxItems: 100 },
    }),
    description: 'Closed read-only snapshot from the primary database for the explicitly selected brand. The three count fields in each coverage row sum exactly to the corresponding snapshot totals; issue_count never exceeds reference_count. Source rows are complete or the request fails; the 100-item issue sample may be truncated while totals and fingerprint cover every issue. The fingerprint is observational evidence, not a repair token.',
    allOf: [
      {
        if: { properties: { consistent: { const: true } }, required: ['consistent'] },
        then: { properties: { issue_count: { const: '0' }, issues_truncated: { const: false }, issues: { maxItems: 0 } } },
      },
      {
        if: { properties: { issue_count: { const: '0' } }, required: ['issue_count'] },
        then: { properties: { consistent: { const: true }, issues_truncated: { const: false }, issues: { maxItems: 0 } } },
      },
      {
        if: { properties: { issue_count: { not: { const: '0' } } }, required: ['issue_count'] },
        then: { properties: { consistent: { const: false }, issues: { minItems: 1 } } },
      },
      {
        if: { properties: { issues_truncated: { const: true } }, required: ['issues_truncated'] },
        then: {
          properties: {
            issue_count: { pattern: '^(?:1(?:0[1-9]|[1-9][0-9])|[2-9][0-9]{2,}|[1-9][0-9]{3,})$' },
            issues: { minItems: 100, maxItems: 100 },
          },
        },
      },
      {
        if: { properties: { issues_truncated: { const: false } }, required: ['issues_truncated'] },
        then: { properties: { issue_count: { pattern: '^(0|[1-9][0-9]?|100)$' } } },
      },
    ],
  },
};

export const operations = [{
  method: 'GET',
  path: '/api/v1/admin/reconciliations/business-inventory',
  operationId: 'getBrandBusinessInventory',
  summary: 'Read the brand business reference inventory',
  tag: 'finance',
  auth: 'admin',
  brandHeader: true,
  data: ref('FinanceBusinessInventorySnapshot'),
  permissions: ['wallet.view.brand', 'wallet.view.platform'],
  additionalErrorStatuses: [413],
  description: 'Read-only observation for the explicitly selected brand. Requires an admin session, X-Brand-ID, and the explicit wallet.view.brand or wallet.view.platform permission. Accepts no query string or body. Reads one primary-database statement snapshot and performs no repair, wallet mutation, or financial authorization.',
}];
