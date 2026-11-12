import { commissionListParameters } from './commission-list-parameters.mjs';
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
  CommissionPaymentPolicyInput: obj({ version, enabled: { type: 'boolean' }, reason: ref('Reason') }, ["version", "enabled"]),
  CommissionPayment: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), run_id: ref('UUID'),
      state: { type: 'string', enum: ['awaiting_approval', 'paying', 'paid', 'failed', 'stale', 'blocked'] },
      payout_mode: { type: 'string', enum: ['manual', 'automatic', 'mixed', 'none'] }, version,
      evidence_epoch: ref('NonnegativeInt64String'), total_points: count, paid_points: count,
      target_count: count, paid_count: count, creation_audit_log_id: ref('UUID'),
      last_error_code: nullable({ type: 'string' }), created_at: ref('DateTime'), updated_at: ref('DateTime'),
    }),
    description: 'Payout progress projection. Exact totals and counts are decimal strings. Beneficiary details are available from the separate audited beneficiaries endpoint. payout_mode is the immutable historical bet snapshot identity and remains mixed. Awaiting approval is valid for manual or mixed mode only when paid_points and paid_count are both zero and last_error_code is null. A mixed cycle with any manual bet snapshot requires brand administrator approval for the entire cycle; its automatic portion is never split out. Blocked correction or evidence records are not approvable.',
  },
  CommissionPaymentHistoryRecord:obj({id:ref('UUID'),actor_type:{type:'string',enum:['admin','system']},actor_id:nullable(ref('UUID')),operation:{type:'string',enum:['create','approve','retry','post','complete','invalidate','fail','adjustment']},state:nullable({type:'string',enum:['awaiting_approval','paying','paid','failed','stale','blocked']}),created_at:ref('DateTime')}),
  CommissionPaymentHistoryPage:obj({brand_id:ref('UUID'),payment_id:ref('UUID'),...pageFields('CommissionPaymentHistoryRecord')}),
  CommissionPaymentBeneficiary: obj({id:ref('UUID'),brand_id:ref('UUID'),payment_id:ref('UUID'),agent_id:ref('UUID'),member_id:ref('UUID'),target_id:nullable(ref('UUID')),original_points:ref('NonnegativePoints'),adjusted_points:nullable(ref('NonnegativePoints')),state:{type:'string',enum:['pending','paid']},paid_at:nullable(ref('DateTime'))}),
  CommissionPaymentBeneficiaryPage:obj({brand_id:ref('UUID'),payment_id:ref('UUID'),...pageFields('CommissionPaymentBeneficiary')}),
  CommissionPaymentPage: obj({ brand_id: ref('UUID'), ...pageFields('CommissionPayment') }),
  CommissionPaymentAction: obj({ version, reason: ref('Reason') }, ["version"]),
};
export const operations = [
  admin('GET', '/commission-payment-policy', 'getCommissionPaymentPolicy', 'Read commission payout policy', ref('CommissionPaymentPolicy')),
  admin('PUT', '/commission-payment-policy', 'updateCommissionPaymentPolicy', 'Update commission payout policy', ref('CommissionPaymentPolicy'), {
    permissions: [], requestBody: ref('CommissionPaymentPolicyInput'),
    description: 'Always returns 403. The payout run switch is managed with platform commission settings; brands inherit it.',
  }),
  admin('GET', '/commission-payments', 'listCommissionPayments', 'List commission payout records', ref('CommissionPaymentPage'), { parameters: [...page,...commissionListParameters('payment')] }),
  admin('GET','/commission-payments/{id}/history','listCommissionPaymentHistory','Read payout operation history',ref('CommissionPaymentHistoryPage'),{parameters:page,description:'Brand-scoped projection of immutable payout audit events. Does not expose raw before/after JSON, IP addresses or worker data. Includes creation, approval, retry, postings, completion, evidence invalidation and manual adjustments. Adjustment events have a null payout state.'}),
  admin('GET','/commission-payments/{id}/beneficiaries','listCommissionPaymentBeneficiaries','Read planned beneficiaries and actual payout progress',ref('CommissionPaymentBeneficiaryPage'),{parameters:[...page,...commissionListParameters('target')],description:'Reads earnings from this payment’s saved run, including before approval. id identifies the earning; target_id, adjusted_points and paid_at are nullable until actual execution. No target, ledger entry or adjustment is created by reading. Filtered count and page share one SQL snapshot.'}),
  admin('GET', '/commission-payments/{id}', 'getCommissionPayment', 'Read a commission payout record', ref('CommissionPayment')),
  admin('POST', '/commission-payments/{id}/approve', 'approveCommissionPayment', 'Approve a manual or mixed commission payout', ref('CommissionPayment'), {
    permissions: ['commission_payment.approve.brand'], parameters:[actorHeader], requestBody: ref('CommissionPaymentAction'), idempotency: true,
    description: 'Requires commission view plus commission_payment.approve.brand and denies super-admin writes. A single brand administrator explicitly approves the whole manual or mixed cycle; the worker performs actual target ledger credits. Mixed payout_mode remains unchanged as historical snapshot identity.',
  }),
  admin('POST', '/commission-payments/{id}/retry', 'retryCommissionPayment', 'Retry a commission payout', ref('CommissionPayment'), {
    permissions: ['commission_payment.retry.brand'], parameters:[actorHeader], requestBody: ref('CommissionPaymentAction'), idempotency: true,
    description: 'Requires commission view plus commission_payment.retry.brand and denies super-admin writes. Failed mixed jobs may use the existing retry action only after persisted approval; evidence is rechecked. Corrections after any paid target remain blocked for manual handling; a payment with no paid targets may become stale.',
  }),
];
