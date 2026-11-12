const ref = name => ({ $ref: `#/components/schemas/${name}` });
const object = properties => ({ type: 'object', additionalProperties: false, required: Object.keys(properties), properties });

export const filterOptionKindsByResource = {
  recharge: ['member'],
  withdrawal: ['member'],
  bet: ['game'],
  report_betting: ['game', 'member'],
  report_ledger: ['member'],
  report_withdrawal: ['member'],
  report_commission: ['agent', 'member', 'cycle'],
  report_attribution: ['game', 'agent', 'member'],
  report_rewards: ['member', 'reward_order'],
  report_reward_orders: ['member', 'reward_order'],
  bet_policy: ['game'],
  withdrawal_policy: ['game'],
  period: ['game', 'period'],
  draw: ['game', 'period'],
  audit: ['actor', 'audit_resource'],
  commission: ['agent', 'member', 'cycle', 'bet_order'],
  reward: ['reward_order', 'member'],
};
const permissionResources = [...new Set(Object.keys(filterOptionKindsByResource).map(resource =>
  resource === 'report_rewards' || resource === 'report_reward_orders' ? 'report_reward' : resource))];
const limit = { type: 'integer', minimum: 1, maximum: 100 };
const offset = { type: 'integer', minimum: 0, maximum: 1000000 };

export const schemas = {
  FilterOption: object({ id: ref('UUID'), name: { type: 'string', description: 'Display name for the selected entity. Treat as plain text; retain id as the filter value.' } }),
  FilterOptionsPage: object({ brand_id: ref('UUID'), items: { type: 'array', items: ref('FilterOption'), maxItems: 100 }, limit, offset }),
};

export const operations = ['admin', 'platform'].map(entry => ({
  method: 'GET', path: `/api/v1/${entry}/filter-options`,
  operationId: entry === 'admin' ? 'adminListFilterOptions' : 'platformListFilterOptions',
  summary: 'List brand-scoped filter options for an authorized target screen',
  tag: 'administration', auth: 'admin', brandHeader: true, data: ref('FilterOptionsPage'),
  parameters: [
    { name: 'resource', in: 'query', required: true, schema: { type: 'string', enum: Object.keys(filterOptionKindsByResource) }, description: 'Target screen whose view permission is required. report_rewards and report_reward_orders both require report_reward.view in the entry scope.' },
    { name: 'kind', in: 'query', required: true, schema: { type: 'string', enum: [...new Set(Object.values(filterOptionKindsByResource).flat())] }, description: 'Allowed combinations: ' + Object.entries(filterOptionKindsByResource).map(([resource, kinds]) => `${resource}: ${kinds.join(', ')}`).join('; ') + '.' },
    { name: 'action', in: 'query', schema: { type: 'string', enum: ['view', 'grant'], default: 'view' }, description: 'grant is supported only for reward/member and requires reward.grant.brand; super administrators cannot grant.' },
    { name: 'search', in: 'query', schema: { type: 'string' }, description: 'Account prefix (at least two characters), case insensitive and trimmed; required for kind=member, agent or actor; unsupported for other kinds. Filtering happens before pagination.' },
    { name: 'limit', in: 'query', schema: { ...limit, default: 50 } },
    { name: 'offset', in: 'query', schema: { ...offset, default: 0 } },
  ],
  permissions: [...permissionResources.map(resource => `${resource}.view.${entry === 'admin' ? 'brand' : 'platform'}`), ...(entry === 'admin' ? ['reward.grant.brand'] : [])],
  successHeaders: { 'Cache-Control': { description: 'Filter options must not be cached.', schema: { type: 'string', const: 'no-store' } } },
  description: 'Requires a valid selected brand in X-Brand-ID and the named resource view grant (or reward.grant.brand for action=grant); the listed grants are alternatives selected by resource, not cumulative requirements. Options inherit target-screen authorization and do not require an additional user or agent browsing grant. Each resource/kind pair must be one of the documented combinations. Unknown or repeated query parameters and invalid pagination return 400 REQUEST_INVALID; an invalid selected brand also returns 400 REQUEST_INVALID. Missing permission or brand scope returns 403. Options are ordered by display name then id, within the selected brand; bet game options contain the selected brand\'s configured order games, including historical offline or deleted games; bet_policy game options contain only connected, non-deleted platform games; period, draw and withdrawal_policy options contain only brand custom games and their periods; report game options include custom games and connected platform games for historical queries. limit defaults to 50 and offset to 0, including when their query values are empty. The response has no total count or has-more field; an empty items array is valid. Reads record filter.options.view audit before returning data; query or audit failure returns 503 SERVICE_UNAVAILABLE without a successful result. This endpoint does not modify business records or balances.',
}));
