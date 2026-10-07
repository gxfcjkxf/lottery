const ref = name => ({ $ref: `#/components/schemas/${name}` });
const obj = (properties, required = Object.keys(properties)) => ({ type: 'object', properties, required, additionalProperties: false });
const nullable = schema => ({ anyOf: [schema, { type: 'null' }] });
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$' };
const page = [
  { name: 'limit', in: 'query', required: false, schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
  { name: 'offset', in: 'query', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
];
export const schemas = {
  CommissionDiscovery: obj({
    id: ref('UUID'), brand_id: ref('UUID'),
    state: { type: 'string', enum: ['pending', 'registered', 'failed'] },
    version: { type: 'integer', minimum: 1, maximum: 9007199254740991 },
    cycle_id: nullable(ref('UUID')),
    window_from: nullable(ref('DateTime')), window_to: nullable(ref('DateTime')),
    next_check_at: ref('DateTime'), last_error_code: nullable({ type: 'string' }),
    last_audit_log_id: nullable(ref('UUID')), created_at: ref('DateTime'), updated_at: ref('DateTime'),
  }),
  CommissionDiscoveryPage: obj({
    brand_id: ref('UUID'), items: { type: 'array', items: ref('CommissionDiscovery'), maxItems: 100 },
    total_count: count, limit: { type: 'integer', minimum: 1, maximum: 100 },
    offset: { type: 'integer', minimum: 0, maximum: 1000000 },
  }),
};

const admin = (method, suffix, operationId, summary, data, extra = {}) => ({
  method, path: `/api/v1/admin/commission-discovery${suffix}`, operationId, summary,
  tag: 'commissions', auth: 'admin', brandHeader: true, data,
  permissions: method === 'GET' ? ['commission.view.brand', 'commission.view.platform'] : ['commission.retry.brand'],
  ...extra,
});

export const operations = [
  admin('GET', '', 'listCommissionDiscoveries', 'List commission discovery records', ref('CommissionDiscoveryPage'), {
    parameters: page,
    description: 'Primary-only, audited, current-session brand/platform view. Strict limit and offset pagination; empty, duplicate and unknown query parameters are rejected. Listing does not create commission cycles.',
  }),
  admin('POST', '/{id}/retry', 'retryCommissionDiscovery', 'Retry commission discovery', ref('CommissionDiscovery'), {
    requestBody: ref('CommissionCycleRetry'), idempotency: true,
    description: 'Requires current commission view plus exact commission.retry.brand and denies super-admin writes. Uses the cycle retry version/reason shape. Rechecks current session, permission and brand state before receipt replay; request bytes are fingerprinted exactly. Transient lock contention returns retryable 503 without caching a terminal result. Retry does not perform a payout or commission calculation.',
  }),
];
