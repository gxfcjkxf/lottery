import { readFileSync } from 'node:fs';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());
describe('member account navigation', () => {
  it('places points directly below the account and uses the simple bilingual label', () => {
    const source = readFileSync(new URL('./Page.vue', import.meta.url), 'utf8');
    const details = source.match(/<dl class="account-details">([\s\S]*?)<\/dl>/)![1];
    expect(details.indexOf("ui('user.auth.accountLabel')")).toBeLessThan(details.indexOf("ui('user.account.points')"));
    expect(details.indexOf("ui('user.account.points')")).toBeLessThan(details.indexOf("ui('common.phone')"));
    for (const [locale, label] of [['en', 'Points'], ['zh-CN', '积分']] as const) {
      const i18n = createUserI18n(); i18n.setLocale(locale);
      expect(i18n.t('user.account.points')).toBe(label);
    }
    expect(details).not.toContain('user.account.availablePoints');
  });
  for (const locale of ['en', 'zh-CN']) {
    it(`keeps referral and help tools without duplicate primary navigation or agent access (${locale})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: Page }] });
      await router.push('/account');
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      const tools = html.match(/<nav\b[^>]*class="account-tools"[^>]*>([\s\S]*?)<\/nav>/)![1];
      expect(tools).toContain('href="/invites"');
      expect(tools).toContain(locale === 'en' ? 'Referral codes' : '推荐码');
      expect(tools).toContain('href="/help"');
      for (const path of ['/agent', '/results', '/orders', '/wallet', '/notifications']) expect(tools).not.toContain(`href="${path}"`);
      expect(html).not.toContain('href="/agent"');
      expect(html).not.toContain('class="auth-error"');
      expect(html).not.toMatch(/class="detail-panel"[^>]*>\s*(正在读取|Loading|Reading)/);
    });
  }
  it('removes member agent route and keeps game-scoped result links', () => {
    expect(readFileSync(new URL('./main.ts', import.meta.url), 'utf8')).not.toContain('path: "/agent"');
    for (const file of ['BettingPanel.vue']) {
      const source = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8');
      expect(source).toContain('game-results-link');
      expect(source).toMatch(/(?:platform_game_id|game_id): game.id/);
    }
  });
});
