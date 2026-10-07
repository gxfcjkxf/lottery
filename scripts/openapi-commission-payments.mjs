const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$' };
const version = { type: 'integer', minimum: 1, maximum: 9007199254740991 };
const actorHeader = {name:'X-Commission-Payment-Actor-ID',in:'header',required:true,schema:ref('UUID'),description:'Administrator UUID captured when the financial request was reviewed; must equal the freshly authenticated actor even on receipt replay.'};
const page = [
  { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
const pageFields = items => ({ items: { type: 'array', items: ref(items), maxItems: 100 }, total_count: count, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } });
const admin = (method, suffix, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin${suffix}`, operationId, summary, tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: ['commission.view.brand', 'commission.view.platform'],
  description: 'Primary-only audited brand/platform read. Payout policy starts disabled; payment targets and private wallet or worker details are omitted.', ...extra,
});
export const schemas = {
  CommissionPaymentPolicy: obj({ brand_id: ref('UUID'), version, enabled: { type: 'boolean' }, audit_log_id: { anyOf: [ref('UUID'), { type: 'string', const: '' }] }, updated_at: ref('DateTime') }),
  CommissionPaymentPolicyInput: obj({ version, enabled: { type: 'boolean' }, reason: ref('Reason') }),
  CommissionPayment: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), run_id: ref('UUID'),
      state: { type: 'string', enum: ['awaiting_approval', 'paying', 'paid', 'failed', 'stale', 'blocked'] },
      payout_mode: { type: 'string', enum: ['manual', 'automatic', 'mixed', 'none'] }, version,
      evidence_epoch: ref('NonnegativeInt64String'), total_points: count, paid_points: count,
      target_count: count, paid_count: count, creation_audit_log_id: ref('UUID'),
      last_error_code: nullable({ type: 'string' }), created_at: ref('DateTime'), updated_at: ref('DateTime'),
    }),
    description: 'Payout progress projection. Exact totals and counts are decimal strings. Payment targets are never exposed. Mixed mode remains blocked pending explicit human confirmation; corrections after any credited target require manual handling.',
  },
  CommissionPaymentPage: obj({ brand_id: ref('UUID'), ...pageFields('CommissionPayment') }),
  CommissionPaymentAction: obj({ version, reason: ref('Reason') }),
};
export const operations = [
  admin('GET', '/commission-payment-policy', 'getCommissionPaymentPolicy', 'Read commission payout policy', ref('CommissionPaymentPolicy')),
  admin('PUT', '/commission-payment-policy', 'updateCommissionPaymentPolicy', 'Update commission payout policy', ref('CommissionPaymentPolicy'), {
    permissions: ['commission_payment_policy.write.brand'], parameters:[actorHeader], requestBody: ref('CommissionPaymentPolicyInput'), idempotency: true,
    description: 'Requires commission view plus commission_payment_policy.write.brand; super-admin writes are denied. The default is disabled and enabling requires an explicit version and audit reason. Session and permission are checked before receipt replay.',
  }),
  admin('GET', '/commission-payments', 'listCommissionPayments', 'List commission payout records', ref('CommissionPaymentPage'), { parameters: page }),
  admin('GET', '/commission-payments/{id}', 'getCommissionPayment', 'Read a commission payout record', ref('CommissionPayment')),
  admin('POST', '/commission-payments/{id}/approve', 'approveCommissionPayment', 'Approve a manual commission payout', ref('CommissionPayment'), {
    permissions: ['commission_payment.approve.brand'], parameters:[actorHeader], requestBody: ref('CommissionPaymentAction'), idempotency: true,
    description: 'Requires commission view plus commission_payment.approve.brand and denies super-admin writes. A single administrator approves; the worker performs actual target ledger credits. Manual mode waits for approval; automatic mode is system-approved. Mixed mode stays blocked until explicitly confirmed.',
  }),
  admin('POST', '/commission-payments/{id}/retry', 'retryCommissionPayment', 'Retry a commission payout', ref('CommissionPayment'), {
    permissions: ['commission_payment.retry.brand'], parameters:[actorHeader], requestBody: ref('CommissionPaymentAction'), idempotency: true,
    description: 'Requires commission view plus commission_payment.retry.brand and denies super-admin writes. Evidence is rechecked. Corrections after any paid target are blocked for manual handling; a payment with no paid targets may become stale.',
  }),
];
