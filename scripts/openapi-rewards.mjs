const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = properties => ({ type: 'object', properties, required: Object.keys(properties), additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const version = { type: 'integer', minimum: 1, maximum: 9007199254740991 };
const reason = { ...ref('Reason'), description: 'Valid UTF-8, at most 500 bytes, no leading/trailing whitespace or Unicode control characters. Not normalized on write.' };
const actor = { name: 'X-Reward-Actor-ID', in: 'header', required: true, schema: ref('UUID'), description: 'Administrator UUID frozen during review; freshly authenticated actor must match before execution or receipt replay. Not an authorization credential.' };
const page = [
  { name: 'limit', in: 'query', schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
const pageFields = item => ({ brand_id: ref('UUID'), items: { type: 'array', items: ref(item), maxItems: 100 }, total_count: { type: 'string', pattern: '^(0|[1-9][0-9]*)$' }, limit: { type: 'integer', minimum: 1, maximum: 100 }, offset: { type: 'integer', minimum: 0, maximum: 1000000 } });
const when = (key, value, properties) => ({ if: { properties: { [key]: { const: value } }, required: [key] }, then: { properties } });
export const schemas = {
  RewardGrantInput: obj({ member_id: ref('UUID'), points: ref('PositiveInt64String'), reason }),
  RewardActionInput: obj({ version, reason }),
  RewardOrder: {
    ...obj({ id: ref('UUID'), brand_id: ref('UUID'), member_id: ref('UUID'), points: ref('PositiveInt64String'), state: { type: 'string', enum: ['granted', 'revocation_pending', 'revoked'] }, version,
      grant_ledger_entry_id: ref('UUID'), revoke_ledger_entry_id: nullable(ref('UUID')), creation_audit_log_id: ref('UUID'), last_audit_log_id: ref('UUID'),
      last_error_code: nullable({ type: 'string', const: 'REWARD_AVAILABLE_INSUFFICIENT' }), created_by: ref('UUID'), reason, point_policy_version: ref('PositiveInt64String'),
      created_at: ref('DateTime'), updated_at: ref('DateTime'), revoked_at: nullable(ref('DateTime')) }),
    allOf: [
      when('state', 'granted', { version: { const: 1 }, revoke_ledger_entry_id: { type: 'null' }, revoked_at: { type: 'null' }, last_error_code: { type: 'null' } }),
      when('state', 'revocation_pending', { version: { minimum: 2 }, revoke_ledger_entry_id: { type: 'null' }, revoked_at: { type: 'null' }, last_error_code: { const: 'REWARD_AVAILABLE_INSUFFICIENT' } }),
      when('state', 'revoked', { version: { minimum: 2 }, revoke_ledger_entry_id: ref('UUID'), revoked_at: ref('DateTime'), last_error_code: { type: 'null' } }),
    ],
    description: 'Manual reward progress and immutable original grant identity. Points are exact positive int64 strings, not money or current wallet balance. Revocation reverses the entire original gift.available grant; insufficient gift.available persists revocation_pending without moving another source or frozen points. Later funds never automatically resume it. Raw wallet/ledger snapshots and private account IDs are omitted.',
  },
  RewardOrderPage: obj(pageFields('RewardOrder')),
  RewardOrderAction: {
    ...obj({ id: ref('UUID'), brand_id: ref('UUID'), order_id: ref('UUID'), version, operation: { type: 'string', enum: ['grant', 'revoke', 'retry'] },
      state_before: nullable({ type: 'string', enum: ['granted', 'revocation_pending'] }), state_after: { type: 'string', enum: ['granted', 'revocation_pending', 'revoked'] },
      actor_id: ref('UUID'), reason, audit_log_id: ref('UUID'), ledger_entry_id: nullable(ref('UUID')), created_at: ref('DateTime') }),
    allOf: [
      when('operation', 'grant', { version: { const: 1 }, state_before: { type: 'null' }, state_after: { const: 'granted' } }),
      when('operation', 'revoke', { version: { minimum: 2 }, state_before: { const: 'granted' }, state_after: { enum: ['revocation_pending', 'revoked'] } }),
      when('operation', 'retry', { version: { minimum: 2 }, state_before: { const: 'revocation_pending' }, state_after: { enum: ['revocation_pending', 'revoked'] } }),
      when('state_after', 'revocation_pending', { ledger_entry_id: { type: 'null' } }),
      { if: { properties: { state_after: { enum: ['granted', 'revoked'] } }, required: ['state_after'] }, then: { properties: { ledger_entry_id: ref('UUID') } } },
    ],
    description: 'Immutable administrator action. Pending shortage actions have no ledger entry; grants and completed full revocations bind their actual ledger. History is not current wallet balance.',
  },
  RewardOrderActionPage: obj({ ...pageFields('RewardOrderAction'), order_id: ref('UUID') }),
};
const read = (suffix, operationId, summary, schema, parameters = []) => ({ method: 'GET', path: `/api/v1/admin/reward-orders${suffix}`, operationId, summary, tag: 'rewards', auth: 'admin', brandHeader: true, permissions: ['reward.view.brand', 'reward.view.platform'], parameters, data: ref(schema), description: 'Primary-only repeatable-read administrative query with current session/brand permission and committed query audit. Strict query/body rejection; no grant, revoke or automatic retry. Super-admin is read-only.' });
const write = (suffix, operationId, summary, action, input, successStatus = 200) => ({ method: 'POST', path: `/api/v1/admin/reward-orders${suffix}`, operationId, summary, tag: 'rewards', auth: 'admin', brandHeader: true,
  permissions: [`reward.${action}.brand`], parameters: [actor], requestBody: ref(input), data: ref('RewardOrder'), idempotency: true, successStatus,
  description: 'Requires reward view plus the independent brand-scoped action permission; super-admin writes are forbidden. Version, reason, actor, target and raw original body are frozen for replay. Disabled brand rejects writes; paused brand permits financial handling. Grant is a single operator approval and atomic gift.available credit. Revoke/retry are explicit full original-grant reversals; shortage returns a durable successful revocation_pending receipt, not a retryable HTTP failure. No automatic worker or other-source deduction. Cached receipts describe the original operation; current state is read separately.' });
export const operations = [
  read('', 'listRewardOrders', 'List manual reward orders', 'RewardOrderPage', page),
  write('', 'grantManualReward', 'Grant a manual reward', 'grant', 'RewardGrantInput', 201),
  read('/{id}', 'getRewardOrder', 'Read a manual reward order', 'RewardOrder'),
  read('/{id}/actions', 'listRewardOrderActions', 'Read immutable reward actions', 'RewardOrderActionPage', page),
  write('/{id}/revoke', 'revokeManualReward', 'Revoke the complete manual reward', 'revoke', 'RewardActionInput'),
  write('/{id}/retry-revocation', 'retryManualRewardRevocation', 'Explicitly continue pending reward revocation', 'retry', 'RewardActionInput'),
];
