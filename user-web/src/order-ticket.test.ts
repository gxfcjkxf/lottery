import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import MarkSixOrders from '@lottery/shared/MarkSixOrders.vue';
import CanadaOrders from '@lottery/shared/CanadaOrders.vue';
import type { MarkSixOrder } from '../../shared/src/mark-six';
import type { CanadaOrder } from '../../shared/src/canada';

const common = { id: 'ticket-private-id', period_no: '20261017', status: 'settled', total_points: '9007199254740993', prize_points: '34', refund_points: '12', effective_points: '22' };
describe('compact user ticket cards', () => {
  for (const locale of ['en', 'zh-CN'] as const) {
    it(`uses the shared empty-state primitive for both shared ticket lists (${locale})`, async () => {
      for (const component of [MarkSixOrders, CanadaOrders]) {
        const html = await renderToString(createSSRApp(component, { orders: [], locale }));
        expect(html).toContain('class="empty-state"');
        expect(html).toContain(locale === 'en' ? 'No tickets yet' : '暂无注单');
        expect(html).not.toContain('<article');
      }
    });
    it(`keeps summary content outside collapsed details and all financial detail inside (${locale})`, async () => {
      const marks = { ...common, attributes: {}, items: [
        { code: 'big', points: '11', odds: '1.9', outcome: 'won', payout: '21' },
        { code: 'small', points: '12', odds: '1.9', outcome: 'lost', payout: '0' },
        { code: 'odd', points: '13', odds: '1.9', outcome: 'won', payout: '25' },
      ] } as unknown as MarkSixOrder;
      const canada = { ...common, profile: 'canada_niuniu', reserved_points: '360', lost_points: '120', items: [
        { code: 'a', points: '11', odds: '1.9', risk_points: '110', loss_points: '0', outcome: 'won', payout: '21' },
        { code: 'b', points: '12', odds: '1.9', risk_points: '120', loss_points: '120', outcome: 'lost', payout: '0' },
        { code: 'aa', points: '13', odds: '1.9', risk_points: '260', loss_points: '0', outcome: 'won', payout: '25' },
      ] } as unknown as CanadaOrder;
      for (const [component, order] of [[MarkSixOrders, marks], [CanadaOrders, canada]] as const) {
        const html = await renderToString(createSSRApp(component, { orders: [order], locale, compact: true }));
        const summary = html.slice(0, html.indexOf('<details'));
        expect(summary).toContain(common.period_no);
        expect(summary).toContain(common.total_points);
        expect(summary).toContain('11');
        expect(summary).toContain('12');
        expect(summary).not.toContain(common.id);
        expect(summary).toContain(locale === 'en' ? '+1 more' : '另有 1 项投注');
        expect(html).toMatch(/<details[^>]*>/);
        expect(html).not.toMatch(/<details[^>]*\sopen(?:[\s=>])/);
        const detail = html.slice(html.indexOf('<details'));
        expect(detail).toContain(common.id);
        expect(detail).toContain('34');
        expect(detail).toContain('13');
        expect(detail).toContain(locale === 'en' ? 'Ticket details' : '注单详情');
      }
      const normal = await renderToString(createSSRApp(MarkSixOrders, { orders: [marks], locale }));
      expect(normal.slice(0, normal.indexOf('<details'))).toContain(common.id);
    });
  }
});
