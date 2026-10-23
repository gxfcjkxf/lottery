import { readFileSync } from 'node:fs';
import { createSSRApp, h } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import OrderHistoryLayout from './OrderHistoryLayout.vue';
import { createUserI18n, userI18nKey } from './i18n';

async function render(locale: 'en' | 'zh-CN', loading = false, empty = true) {
  const i18n = createUserI18n();
  i18n.setLocale(locale);
  const context: { teleports?: Record<string, string> } = {};
  const html = await renderToString(createSSRApp({ render: () => h(OrderHistoryLayout, { title: 'Orders', loading, empty }, {
    default: () => h('div', { class: 'order-content' }, 'content'),
    pagination: () => h('nav', { class: 'history-pages' }, 'pages'),
  }) }).provide(userI18nKey, i18n), context);
  expect(html).not.toContain('data-refresh-button');
  return html + context.teleports?.['#user-page-refresh'];
}

describe('one user order history layout', () => {
  for (const locale of ['en', 'zh-CN'] as const) {
    it(`shares the secondary refresh button and localized empty state (${locale})`, async () => {
      const html = await render(locale);
      expect(html).toContain('class="button button-secondary"');
      expect(html).toContain(locale === 'en' ? 'Refresh' : '刷新');
      expect(html).toContain(locale === 'en' ? 'No tickets yet' : '暂无注单');
      expect(html).toContain('order-history-empty');
      expect(html).not.toContain('order-content');
      // Empty later pages must still offer navigation back to earlier tickets.
      expect(html).toContain('history-pages');
    });
  }
  it('does not show an empty state during loading, errors or populated content', async () => {
    const loading = await render('en', true);
    expect(loading).not.toContain('order-history-empty');
    expect(loading).toMatch(/<button[^>]*disabled/);
    for (const html of [loading, await render('en', false, false)]) expect(html).toContain('order-content');
    expect(await render('en', false, false)).not.toContain('order-history-empty');
  });
  it('uses one history layout and centered empty styling', () => {
    for (const name of ['BettingHistory']) {
      const source = readFileSync(new URL(`./${name}.vue`, import.meta.url), 'utf8');
      expect(source).toMatch(/import OrderHistoryLayout from ['"]\.\/OrderHistoryLayout.vue['"]/);
      expect(source).toContain('<OrderHistoryLayout');
      expect(source).toContain(':empty="');
    }
    const layout = readFileSync(new URL('./OrderHistoryLayout.vue', import.meta.url), 'utf8');
    expect(layout).toContain('class="order-history-empty empty-state"');
    expect(layout).not.toMatch(/\.order-history-empty\s*\{/);
    expect(layout).toMatch(/\.order-history\s*\{[^}]*gap: 12px;/);
  });
});
