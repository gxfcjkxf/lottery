import { describe, expect, it } from 'vitest';
import { notificationCatalog } from '@lottery/shared';
import { formatIntegerString, notificationAmount, renderNotification, type NotificationPresentationItem } from './notification-presentation';
import type { NotificationEventType } from './notification-api';

function item(event: NotificationEventType, points: string | null = '489.9'): NotificationPresentationItem {
  return {
    event_type: event, template_key: event, template_version: 1,
    content: notificationCatalog[event],
    payload: { resource_id: 'business-reference', points: event === 'member.joined' || event.startsWith('draw.') ? null : points },
    created_at: '2026-11-06T14:32:27Z',
  };
}

describe('notification presentation', () => {
  it.each(['en', 'zh'] as const)('renders concise bilingual system events in %s', locale => {
    for (const event of Object.keys(notificationCatalog) as NotificationEventType[]) {
      if (event.startsWith('draw.')) continue;
      const rendered = renderNotification(item(event), locale);
      expect(rendered.title.length).toBeGreaterThan(0);
      expect(rendered.body.length).toBeGreaterThan(0);
      expect(rendered.protectedNote).toBeNull();
      expect(rendered.body).not.toMatch(/Historical record|this record is retained|历史记录|此记录会保留/);
      if (event !== 'member.joined') {
        expect(rendered.body).toContain('489.9');
        expect(notificationAmount(item(event), locale)?.label.length).toBeGreaterThan(0);
      }
    }
  });

  it.each([
    ['bet.order.won', '489.9', '+489.9', 'positive'],
    ['bet.order.prize_reversed', '489.9', '-489.9', 'negative'],
    ['reward.order.granted', '500', '+500', 'positive'],
    ['reward.order.revoked', '500', '-500', 'negative'],
    ['bet.order.cancelled', '10', '+10', 'positive'],
    ['bet.order.judged_cancelled', '10', '+10', 'positive'],
    ['recharge.confirmed', '25', '+25', 'positive'],
    ['commission.paid', '25', '+25', 'positive'],
    ['commission.adjusted', '-42.5', '-42.5', 'negative'],
    ['commission.corrected', '42.5', '+42.5', 'positive'],
    ['commission.corrected', '-42', '-42', 'negative'],
    ['bet.order.placed', '10', '10', 'neutral'],
    ['bet.order.abnormal', '10', '10', 'neutral'],
    ['withdrawal.order.paid', '10', '10', 'neutral'],
    ['withdrawal.order.rejected', '10', '10', 'neutral'],
    ['commission.corrected', '0', '0', 'positive'],
    ['commission.paid', '900719925474099312345', '+900,719,925,474,099,312,345', 'positive'],
  ] as const)('shows the event amount without implying a wallet balance: %s %s', (event, raw, value, tone) => {
    expect(notificationAmount(item(event, raw), 'en')).toMatchObject({ value, tone });
  });

  it('preserves operator copy and replaces placeholders without adding duplicate explanations', () => {
    const custom = { ...item('commission.corrected', '-42.5'), template_version: 2, content: {
      en: { title: 'Correction {points}', body: 'Adjustment {points}; reference {resource_id}.' },
      'zh-CN': { title: '更正 {points}', body: '调整 {points}；编号 {resource_id}。' },
    } };
    expect(renderNotification(custom, 'en')).toMatchObject({ title: 'Correction -42.5', body: 'Adjustment -42.5; reference business-reference.', protectedNote: null });
    expect(renderNotification(custom, 'zh').body).toBe('调整 -42.5；编号 business-reference。');
    expect(notificationAmount(custom, 'zh')).toMatchObject({ label: '调整积分', value: '-42.5' });
  });

  it('preserves saved draw facts and leading zeroes', () => {
    const draw = { ...item('draw.result.corrected'), payload: { resource_id: 'draw-id', points: null, draw: {
      game_id: 'game-id', period_id: 'period-id', period_no: '2026310', result: { regular: [], special: [], digits: [0, 1, 0] },
      drawn_at: '2026-11-06T12:00:00Z', previous_draw_id: 'previous-id',
    } } };
    expect(renderNotification(draw, 'en').protectedNote).toContain('Digits: 010');
    expect(renderNotification(draw, 'zh').protectedNote).toContain('数字：010');
    expect(notificationAmount(draw, 'zh')).toBeNull();
    expect(() => renderNotification(item('draw.result.published'), 'en')).toThrow(/draw facts/);
  });

  it('rejects unsupported events, mismatched templates and missing snapshots', () => {
    expect(() => renderNotification({ ...item('member.joined'), event_type: 'unknown' as never, template_key: 'unknown' as never }, 'en')).toThrow(RangeError);
    expect(() => renderNotification({ ...item('member.joined'), template_key: 'bet.order.placed' }, 'en')).toThrow(RangeError);
    expect(() => renderNotification({ ...item('commission.paid'), content: null as never }, 'en')).toThrow(/immutable content snapshot/);
    expect(() => renderNotification({ ...item('commission.paid'), template_version: 0 }, 'en')).toThrow(RangeError);
    expect(() => renderNotification(item('commission.paid', null), 'en')).toThrow(/Invalid points/);
  });

  it('formats integer strings without losing precision and preserves decimals', () => {
    expect(formatIntegerString('900719925474099312345', 'en')).toBe('900,719,925,474,099,312,345');
    expect(formatIntegerString('-1200000', 'zh')).toBe('-1,200,000');
    expect(formatIntegerString(null, 'en')).toBe('');
    expect(formatIntegerString('489.9', 'en')).toBe('489.9');
  });
});
