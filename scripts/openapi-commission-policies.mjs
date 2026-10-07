const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const version = { type: 'integer', minimum: 1, description: 'Positive PostgreSQL bigint version; browser clients reject versions beyond JavaScript safe integer range instead of rounding. SQL enforces the int64 bound.' };
const calendarFields = {
  timezone: { type: 'string', minLength: 1, description: 'Valid IANA zone or UTC, never machine Local.' },
  cycle: { type: 'string', enum: ['weekly', 'monthly'] },
  boundary_time: { type: 'string', pattern: '^([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]$' },
  weekday: nullable({ type: 'integer', minimum: 0, maximum: 6 }),
  month_day: nullable({ type: 'integer', minimum: 1, maximum: 31 }),
  short_month: { type: 'string', enum: ['', 'last_day', 'skip'] },
};
export const schemas = {
  CommissionCalendar: {
    ...obj(calendarFields),
    oneOf: [
      { properties: { cycle: { const: 'weekly' }, weekday: { type: 'integer', minimum: 0, maximum: 6 }, month_day: { type: 'null' }, short_month: { const: '' } } },
      { properties: { cycle: { const: 'monthly' }, weekday: { type: 'null' }, month_day: { type: 'integer', minimum: 1, maximum: 31 }, short_month: { enum: ['last_day', 'skip'] } } },
    ],
    description: 'Explicit local calendar. DST-ambiguous or nonexistent boundaries are rejected by the calendar calculation. Half-open UTC cycles use bet placement time, not settlement time.',
  },
  CommissionPolicyConfig: {
    ...obj({ enabled: { type: 'boolean' }, calendar: nullable(ref('CommissionCalendar')), payout_mode: { type: 'string', enum: ['manual', 'automatic'] } }),
    allOf: [{ if: { properties: { enabled: { const: true } }, required: ['enabled'] }, then: { properties: { calendar: ref('CommissionCalendar') } } }],
    description: 'Initial policy is disabled/null calendar/manual. Enabling requires enabled agency management and matching weekly/monthly cycle. Automatic is a saved choice, not an implemented payout worker.',
  },
  CommissionPolicy: obj({ brand_id: ref('UUID'), version, config: ref('CommissionPolicyConfig'), created_at: ref('DateTime'), updated_at: ref('DateTime'), revision_id: ref('UUID'), audit_log_id: ref('UUID') }, ['brand_id', 'version', 'config', 'created_at', 'updated_at', 'revision_id']),
  CommissionPolicyInput: obj({ version, config: ref('CommissionPolicyConfig'), reason: ref('Reason') }),
  CommissionPolicyRevision: obj({ id: ref('UUID'), brand_id: ref('UUID'), version, config: ref('CommissionPolicyConfig'), changed_by: nullable(ref('UUID')), reason: ref('Reason'), audit_log_id: nullable(ref('UUID')), created_at: ref('DateTime') }),
  CommissionPolicyHistory: obj({ items: { type: 'array', items: ref('CommissionPolicyRevision'), maxItems: 100 }, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } }),
};
const admin = (method, path, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin${path}`, operationId, summary, tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: method === 'GET' ? ['commission_policy.view.brand', 'commission_policy.view.platform'] : ['commission_policy.write.brand'],
  ...extra,
});
export const operations = [
  admin('GET', '/commission-policy', 'getCommissionPolicy', 'Read brand financial commission policy', ref('CommissionPolicy'), { description: 'Audited read for selected brand. Super-admin requires platform view grant and cannot write. This policy does not create or pay a commission batch.' }),
  admin('PUT', '/commission-policy', 'updateCommissionPolicy', 'Replace financial commission policy', ref('CommissionPolicy'), {
    requestBody: ref('CommissionPolicyInput'), idempotency: true,
    description: 'Complete replacement with version/reason. Unknown, duplicate, null and missing required fields are rejected. Rechecks actual session and brand write permission before cached receipt replay; changed body with same key conflicts. Audit, immutable revision and current policy commit together. Enabled calendar must match enabled agency cycle; disable financial policy first before changing agency cycle. Changes affect only new bet snapshots; legacy NULL is not backfilled. No batch generation or payout is performed.',
  }),
  admin('GET', '/commission-policy/history', 'listCommissionPolicyHistory', 'Read immutable financial policy history', ref('CommissionPolicyHistory'), {
    parameters: [
      { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
      { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
    ], description: 'Descending immutable revisions scoped to selected brand. Initial disabled revision has null changed_by/audit_log_id; later revisions reference the matching admin audit.',
  }),
];
