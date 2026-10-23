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
    description: 'Initial policy is disabled/null calendar/manual. The financial calendar is the single source for the weekly/monthly cycle. Automatic is a saved choice, not an implemented payout worker.',
  },
  CommissionPolicy: obj({ brand_id: ref('UUID'), version, config: ref('CommissionPolicyConfig'), created_at: ref('DateTime'), updated_at: ref('DateTime'), revision_id: ref('UUID'), audit_log_id: ref('UUID') }, ['brand_id', 'version', 'config', 'created_at', 'updated_at', 'revision_id']),
  CommissionPolicyInput: obj({ version, config: ref('CommissionPolicyConfig'), reason: ref('Reason') }, ["version", "config"]),
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
    description: 'Complete replacement with an optimistic concurrency version. Unknown, duplicate, null and missing required fields are rejected. Rechecks actual session and brand write permission before cached receipt replay; changed body with same key conflicts. Only the current policy is replaced; no policy revision or change-history audit is written. Saving a calendar atomically synchronizes the derived agency snapshot cycle and advances its version when the cycle changes. Changes affect only new bet snapshots; every new bet records its current policy. No batch generation or payout is performed.',
  }),
];
