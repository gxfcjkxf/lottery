const ref = name => ({ $ref: `#/components/schemas/${name}` });
const object = properties => ({ type: 'object', properties, required: Object.keys(properties), additionalProperties: false });
const text = { type: 'string' };
const count = { type: 'string', pattern: '^(0|[1-9][0-9]*)$' };
const date = { type: 'string', format: 'date' };

export const schemas = {
  BetFrequencyGames: object({ items: { type: 'array', items: object({ id: ref('UUID'), name: text }) } }),
  BetFrequencyPage: object({
    effective_item_count: count,
    play_count: count,
    items: {
      type: 'array', maxItems: 100,
      items: object({ play_id: text, label: object({ en: text, 'zh-CN': text }), effective_item_count: count }),
    },
  }),
};

const platformReport = { method: 'GET', tag: 'reports', auth: 'admin', brandHeader: false, permissions: ['report_betting.view.platform'] };
export const operations = [
  {
    ...platformReport, path: '/api/v1/platform/reports/bet-frequency/games', operationId: 'platformListBetFrequencyGames',
    summary: 'List games for the platform betting frequency report', data: ref('BetFrequencyGames'),
    description: 'Lists platform game IDs and names, including historical games, sorted by name then ID. Rejects a brand header or any query parameters.',
  },
  {
    ...platformReport, path: '/api/v1/platform/reports/bet-frequency', operationId: 'platformReadBetFrequency',
    summary: 'Read betting frequency by play for a platform game', data: ref('BetFrequencyPage'),
    parameters: [
      { name: 'game_id', in: 'query', required: true, schema: ref('UUID') },
      { name: 'from', in: 'query', required: true, schema: date },
      { name: 'to', in: 'query', required: true, schema: date },
      { name: 'limit', in: 'query', schema: { type: 'integer', minimum: 1, maximum: 100, default: 20 } },
      { name: 'offset', in: 'query', schema: { type: 'integer', minimum: 0, maximum: 1000000, default: 0 } },
    ],
    description: 'Aggregates all brands in a single game over inclusive platform calendar dates, up to 93 days. Counts only settled top-level items with positive effective points, by placement date. Includes archived orders and returns whole-range totals alongside items ranked by count descending and play ID ascending. Each label comes from the saved rule version of the latest counted placement for that play, with rule ID descending as the tie-breaker. Rejects a brand header and unknown or repeated filters. Returns 404 for an unknown game.',
  },
];
