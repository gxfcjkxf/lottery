const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$' };
const version = { type: 'integer', minimum: 1, maximum: 9007199254740991 };
const signedAmount = { allOf: [ref('Int64String'), { not: { const: '0' } }] };
const actorHeader = { name: 'X-Commission-Payment-Actor-ID', in: 'header', required: true, schema: ref('UUID'), description: 'Administrator UUID captured when the financial request was reviewed; must equal the freshly authenticated actor even on receipt replay.' };
const page = [
  { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
const pageFields = items => ({ items: { type: 'array', items: ref(items), maxItems: 100 }, total_count: count, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } });
const admin = (method, suffix, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin${suffix}`, operationId, summary, tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: ['commission.view.brand', 'commission.view.platform'],
  description: 'Primary-only audited brand/platform read. Target projections omit account, raw rule, and wallet bucket details.', ...extra,
});

export const schemas = {
  CommissionAdjustmentInput: { ...obj({ version, points: ref('NonnegativeInt64String'), reason: ref('Reason') }), description: 'Reason must be valid UTF-8 with no leading or trailing whitespace and at most 500 UTF-8 bytes.' },
  CommissionPaymentTarget: obj({
    id: ref('UUID'), brand_id: ref('UUID'), payment_id: ref('UUID'), earning_id: ref('UUID'), agent_id: ref('UUID'), member_id: ref('UUID'),
    original_points: ref('NonnegativeInt64String'), state: { type: 'string', enum: ['pending', 'paid'] }, ledger_entry_id: nullable(ref('UUID')), paid_at: nullable(ref('DateTime')),
    adjustment_version: nullable(version), adjusted_points: nullable(ref('NonnegativeInt64String')), last_adjustment_id: nullable(ref('UUID')),
  }),
  CommissionPaymentTargetPage: obj({ brand_id: ref('UUID'), payment_id: ref('UUID'), ...pageFields('CommissionPaymentTarget') }),
  CommissionAdjustment: obj({
    id: ref('UUID'), brand_id: ref('UUID'), target_id: ref('UUID'), payment_id: ref('UUID'), version: { ...version, minimum: 2 },
    points_before: ref('NonnegativeInt64String'), points_after: ref('NonnegativeInt64String'), delta_points: signedAmount,
    ledger_entry_id: ref('UUID'), audit_log_id: ref('UUID'), created_by: ref('UUID'), reason: ref('Reason'),
    point_policy_version: ref('PositiveInt64String'), created_at: ref('DateTime'),
  }),
  CommissionAdjustmentPage: obj({ brand_id: ref('UUID'), target_id: ref('UUID'), ...pageFields('CommissionAdjustment') }),
};

export const operations = [
  admin('GET', '/commission-payments/{id}/targets', 'listCommissionPaymentTargets', 'List targets for a commission payment', ref('CommissionPaymentTargetPage'), { parameters: page }),
  admin('GET', '/commission-payment-targets/{id}/adjustments', 'listCommissionTargetAdjustments', 'List manual commission adjustments for a target', ref('CommissionAdjustmentPage'), { parameters: page }),
  admin('POST', '/commission-payment-targets/{id}/adjustments', 'createCommissionTargetAdjustment', 'Correct a paid commission target', ref('CommissionAdjustment'), {
    successStatus: 201, permissions: ['commission_adjustment.write.brand'], parameters: [actorHeader], requestBody: ref('CommissionAdjustmentInput'), idempotency: true,
    description: 'Requires commission.view plus commission_adjustment.write.brand and denies super-admin writes. The version is the current adjustment head; points is the desired nonnegative net commission. Only the signed difference is posted to commission.available. A single administrator is bound to the immutable receipt.',
  }),
];
