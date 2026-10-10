import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import OrderPrize from './OrderPrize.vue';

const settled = '2026-10-10T04:00:00Z';
describe('order prize facts', () => {
  it('does not present unsettled orders as credited', async () => {
    const html = await renderToString(createSSRApp(OrderPrize, { locale: 'en', order: { prize_points: '0', settled_at: null, payout_entry_id: null } }));
    expect(html).not.toContain('Prize points');
    expect(html).not.toContain('Applied');
  });
  it('distinguishes zero settlements from genuine credit records in both languages', async () => {
    for (const [locale, none, applied] of [['en', 'No prize credit recorded', 'Applied'], ['zh', '无中奖积分入账记录', '已入账']] as const) {
      const zero = await renderToString(createSSRApp(OrderPrize, { locale, order: { prize_points: '0', settled_at: settled, payout_entry_id: null } }));
      expect(zero).toContain(none);
      expect(zero).not.toContain(applied);
      const credited = await renderToString(createSSRApp(OrderPrize, { locale, order: { prize_points: '9007199254740993', settled_at: settled, payout_entry_id: '11111111-1111-4111-8111-111111111111' } }));
      expect(credited).toContain(applied);
      expect(credited).toContain('9007199254740993');
      expect(credited).not.toContain(none);
    }
  });
});
