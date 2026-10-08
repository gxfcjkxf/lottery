const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$', description: 'Exact nonnegative decimal integer counter.' };
const aggregate = { type: 'string', pattern: '^(0|[1-9][0-9]*)$', description: 'Exact nonnegative decimal numeric aggregate; database numeric totals may exceed int64.' };
const net = { type: 'string', pattern: '^(0|-?[1-9][0-9]*)$', description: 'Exact signed decimal net aggregate; database numeric totals may exceed int64.' };
const version = { type: 'integer', minimum: 1, maximum: Number.MAX_SAFE_INTEGER, description: 'Positive database int64 version. JavaScript clients must reject values above the safe integer limit rather than round them.' };
const epoch = { type: 'string', format: 'nonnegative-int64-string', pattern: '^(0|[1-9][0-9]*)$', maxLength: 19, description: 'Canonical nonnegative decimal evidence epoch. Clients must register the custom format nonnegative-int64-string to enforce the exact 0..9223372036854775807 int64 bound; generic OpenAPI readers apply only canonical syntax and length here. Never convert to a JavaScript Number.' };
const reason = { type: 'string', minLength: 1, maxLength: 500, pattern: '^(?=\\S)(?![\\s\\S]*[\\u0000-\\u001f\\u007f-\\u009f])(?:[\\s\\S]*\\S)?$', description: 'Valid UTF-8 reason, nonblank, with no leading or trailing whitespace and no Unicode control characters (Unicode category Cc); maximum 500 UTF-8 bytes.' };
const actorHeader = { name: 'X-Commission-Correction-Actor-ID', in: 'header', required: true, schema: ref('UUID'), description: 'Administrator UUID for this reviewed financial action; must match the freshly authenticated actor, including on receipt replay.' };
const page = [
  { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
const pageFields = items => ({ items: { type: 'array', items: ref(items), maxItems: 100 }, total_count: count, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } });
const viewPermissions = ['commission.view.brand', 'commission.view.platform'];
const admin = (method, suffix, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin${suffix}`, operationId, summary, tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: viewPermissions,
  description: 'Primary-only audited brand/platform read. These routes are canonical admin routes and have no brand-path aliases. Every GET rejects a body and ForceQuery. Only collection and target-list routes accept exactly limit and offset; other query keys, duplicate values, and query strings on detail/policy routes are rejected.',
  ...extra,
});
const action = (permission, requestBody, summary) => ({
  permissions: [...viewPermissions, permission], parameters: [actorHeader], requestBody: ref(requestBody), idempotency: true,
  description: `Requires ordinary brand-admin authorization plus commission view and ${permission}; super-admins are read-only. The request is a closed UTF-8 JSON object with no query parameters. Go Unicode control characters are rejected in reason. A version above JavaScript's maximum safe integer fails closed input validation with 400; SDKs should reject it before sending to avoid rounded intent. A version equal to the maximum safe integer passes input decoding, but a domain version increment overflow returns 409. The authenticated session, actor, and permission are checked again after business and audit writes, immediately before commit. A 409 conflict retains the original intent and requires a fresh review before a new key/body. SQLSTATE 55P03 busy maps to 503 and is not cached; retry with the same idempotency key and identical body. Successful receipts return the original action result, not a later live snapshot. ${summary}`,
});

export const schemas = {
  CommissionCorrectionExecutionPolicy: obj({ brand_id: ref('UUID'), version, enabled: { type: 'boolean' }, audit_log_id: { anyOf: [ref('UUID'), { type: 'string', const: '' }] }, updated_at: ref('DateTime') }),
  CommissionCorrectionExecutionPolicyInput: obj({ version, enabled: { type: 'boolean' }, reason }),
  CommissionCorrectionAction: obj({ version, reason }),
  CommissionCorrectionPlan: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), payment_id: ref('UUID'), run_id: ref('UUID'),
      state: { type: 'string', enum: ['planning', 'ready', 'blocked', 'failed', 'stale'] },
      payout_mode: { type: 'string', enum: ['manual', 'automatic', 'mixed', 'none'] }, version,
      evidence_epoch: epoch, before_points: aggregate, calculated_points: aggregate,
      credit_points: nullable(aggregate), debit_points: nullable(aggregate), net_points: nullable(net),
      target_count: count, planned_count: count, creation_audit_log_id: ref('UUID'), last_audit_log_id: ref('UUID'),
      last_error_code: nullable({ type: 'string' }), created_at: ref('DateTime'), updated_at: ref('DateTime'),
    }),
    description: 'A ready plan is only a preview and does not approve or move funds. before_points, calculated_points, credit_points, and debit_points are exact nonnegative numeric aggregates; net_points is the only signed aggregate. Totals may exceed int64. The three correction totals are all null until planning resolves them. Timestamps preserve Go RFC3339Nano precision. last_error_code is null when there is no current error.',
  },
  CommissionCorrectionPlanTarget: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), plan_id: ref('UUID'), agent_id: ref('UUID'), member_id: ref('UUID'),
      original_target_id: nullable(ref('UUID')), earning_id: nullable(ref('UUID')),
      adjustment_version: nullable(version), previous_correction_target_id: nullable(ref('UUID')),
      financial_version: nullable(version), points_before: ref('NonnegativeInt64String'),
      points_after: ref('NonnegativeInt64String'), delta_points: ref('Int64String'),
      creation_audit_log_id: ref('UUID'), created_at: ref('DateTime'),
    }),
    description: 'Privacy-limited saved beneficiary projection. Per-target amounts and signed delta use exact int64 decimal strings; no account, raw rule, or wallet bucket details are exposed. Timestamps preserve nanoseconds.',
  },
  CommissionCorrectionPlanPage: obj({ brand_id: ref('UUID'), ...pageFields('CommissionCorrectionPlan') }),
  CommissionCorrectionPlanTargetPage: obj({ brand_id: ref('UUID'), plan_id: ref('UUID'), ...pageFields('CommissionCorrectionPlanTarget') }),
  CommissionCorrectionExecution: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), cycle_id: ref('UUID'), plan_id: ref('UUID'), run_id: ref('UUID'),
      payout_mode: { type: 'string', enum: ['manual', 'automatic', 'mixed', 'none'] },
      state: { type: 'string', enum: ['awaiting_approval', 'applying', 'completed', 'paused', 'failed', 'stale'] },
      version, plan_version: version, evidence_epoch: epoch,
      credit_points: aggregate, debit_points: aggregate, net_points: net,
      applied_credit_points: aggregate, applied_debit_points: aggregate,
      target_count: count, applied_count: count, paused_plan_target_id: nullable(ref('UUID')),
      last_error_code: nullable({ type: 'string' }), creation_audit_log_id: ref('UUID'), last_audit_log_id: ref('UUID'),
      approved_by: nullable(ref('UUID')), approval_actor_type: nullable({ type: 'string', enum: ['admin', 'system'] }), approval_audit_log_id: nullable(ref('UUID')),
      cycle_hold_active: { type: 'boolean' }, created_at: ref('DateTime'), updated_at: ref('DateTime'),
    }),
    description: 'credit_points, debit_points, applied_credit_points, and applied_debit_points are exact nonnegative numeric aggregates; net_points is the only signed aggregate. Totals may exceed int64 and credit/debit remain separate. A mixed payout mode requires fresh approval. A paused execution records a cycle hold inherited by later runs unless explicitly continued; continuing clears the current hold so a subsequent run can execute normally. Timestamps preserve Go RFC3339Nano precision.',
  },
  CommissionCorrectionExecutionTarget: {
    ...obj({
      id: ref('UUID'), brand_id: ref('UUID'), execution_id: ref('UUID'), plan_target_id: ref('UUID'),
      agent_id: ref('UUID'), member_id: ref('UUID'), points_before: ref('NonnegativeInt64String'),
      points_after: ref('NonnegativeInt64String'), delta_points: ref('Int64String'),
      state: { type: 'string', enum: ['pending', 'applied'] }, ledger_entry_id: nullable(ref('UUID')),
      audit_log_id: nullable(ref('UUID')), financial_version: nullable(version),
      created_at: ref('DateTime'), applied_at: nullable(ref('DateTime')),
    }),
    description: 'Privacy-limited target posting projection. Points and signed delta are exact int64 decimal strings. Posting witness fields and applied_at remain null until applied. Timestamps preserve nanoseconds.',
  },
  CommissionCorrectionExecutionPage: obj({ brand_id: ref('UUID'), ...pageFields('CommissionCorrectionExecution') }),
  CommissionCorrectionExecutionTargetPage: obj({ brand_id: ref('UUID'), execution_id: ref('UUID'), ...pageFields('CommissionCorrectionExecutionTarget') }),
};

export const operations = [
  admin('GET', '/commission-correction-policy', 'getCommissionCorrectionPolicy', 'Read commission correction execution policy', ref('CommissionCorrectionExecutionPolicy')),
  admin('PUT', '/commission-correction-policy', 'updateCommissionCorrectionPolicy', 'Update commission correction execution policy', ref('CommissionCorrectionExecutionPolicy'), {
    ...action('commission_correction_policy.write.brand', 'CommissionCorrectionExecutionPolicyInput', 'The policy starts disabled; enabling is an explicit audited opt-in and does not execute a plan.'),
  }),
  admin('GET', '/commission-correction-plans', 'listCommissionCorrectionPlans', 'List commission correction plans', ref('CommissionCorrectionPlanPage'), { parameters: page }),
  admin('GET', '/commission-correction-plans/{id}', 'getCommissionCorrectionPlan', 'Read a commission correction plan', ref('CommissionCorrectionPlan')),
  admin('GET', '/commission-correction-plans/{id}/targets', 'listCommissionCorrectionPlanTargets', 'List targets in a commission correction plan', ref('CommissionCorrectionPlanTargetPage'), { parameters: page }),
  admin('POST', '/commission-correction-plans/{id}/retry', 'retryCommissionCorrectionPlan', 'Retry commission correction plan preparation', ref('CommissionCorrectionPlan'), {
    ...action('commission_correction.retry.brand', 'CommissionCorrectionAction', 'Plan retry rebuilds preview evidence only and never approves or posts funds.'),
  }),
  admin('GET', '/commission-correction-executions', 'listCommissionCorrectionExecutions', 'List commission correction executions', ref('CommissionCorrectionExecutionPage'), { parameters: page }),
  admin('GET', '/commission-correction-executions/{id}', 'getCommissionCorrectionExecution', 'Read a commission correction execution', ref('CommissionCorrectionExecution')),
  admin('GET', '/commission-correction-executions/{id}/targets', 'listCommissionCorrectionExecutionTargets', 'List targets in a commission correction execution', ref('CommissionCorrectionExecutionTargetPage'), { parameters: page }),
  admin('POST', '/commission-correction-executions/{id}/approve', 'approveCommissionCorrectionExecution', 'Approve a commission correction execution', ref('CommissionCorrectionExecution'), {
    ...action('commission_correction.approve.brand', 'CommissionCorrectionAction', 'Approval is a fresh explicit action; mixed payout mode remains mixed and requires this approval.'),
  }),
  admin('POST', '/commission-correction-executions/{id}/continue', 'continueCommissionCorrectionExecution', 'Continue a paused commission correction execution', ref('CommissionCorrectionExecution'), {
    ...action('commission_correction.continue.brand', 'CommissionCorrectionAction', 'Explicit continuation clears the current cycle hold so subsequent runs can execute normally; without explicit continuation, new draws inherit the hold.'),
  }),
  admin('POST', '/commission-correction-executions/{id}/retry', 'retryCommissionCorrectionExecution', 'Retry a commission correction execution', ref('CommissionCorrectionExecution'), {
    ...action('commission_correction.execute_retry.brand', 'CommissionCorrectionAction', 'Execution retry permission is distinct from plan preparation retry.'),
  }),
];
