import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());

describe('help page number-model guidance', () => {
  for (const locale of ['en', 'zh']) {
    it(`explains configured digit repetition without imposing unique numbers (${locale})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale === 'zh' ? 'zh-CN' : 'en-US'], language: locale === 'zh' ? 'zh-CN' : 'en-US' });
      const paths = ['/', '/help', '/orders', '/wallet', '/results', '/notifications', '/agent', '/invites', '/login', '/account'];
      const router = createRouter({ history: createMemoryHistory(), routes: paths.map(path => ({ path, component: Page })) });
      await router.push('/help');
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      expect(html).toContain(locale === 'en' ? 'digit-position games' : '数字位玩法');
      expect(html).toContain(locale === 'en' ? 'repeated digits such as 111' : '111 这样的重复数字');
      expect(html).toContain(locale === 'en' ? 'server quote' : '服务器报价');
      expect(html).not.toContain(locale === 'en' ? 'Choose at least the required number of unique numbers.' : '选择不少于规定数量的不同号码。');
      const mobileNav = html.match(/<nav[^>]*class="mobile-primary-nav"[^>]*>([\s\S]*?)<\/nav>/)![1];
      expect([...mobileNav.matchAll(/href="([^"]+)"/g)].map(match => match[1])).toEqual(['/', '/orders', '/wallet', '/account']);
      expect(mobileNav).toContain(locale === 'en' ? 'Me' : '个人');
      expect(html).not.toContain('Open navigation');
      expect(html).toContain(locale === 'en' ? 'Help centre' : '帮助中心');
      const help = html.match(/<section[^>]*class="page-section narrow-page help-page"[^>]*>([\s\S]*?)<\/section>/)![1];
      expect(help.match(/<details\b/g)).toHaveLength(6);
      expect(help).toContain(locale === 'en' ? 'How do I place a bet?' : '如何投注？');
      expect(help).toContain(locale === 'en' ? 'Where can I find my bets and results?' : '在哪里查看注单和开奖结果？');
      expect(help).toContain(locale === 'en' ? 'How do I share my referral link?' : '如何分享推荐链接？');
      expect(help).not.toMatch(/mailto:|support@example\.invalid|help-intro|help-symbol|page-heading|<h[12]\b/);
      expect(help).not.toContain(locale === 'en' ? 'Contact support' : '联系支持团队');
      expect(help).toContain(locale === 'en' ? 'Adults only. Set limits. Take breaks.' : '仅限成年人，量力参与，适时休息。');
      expect(help.indexOf('help-responsible')).toBeGreaterThan(help.lastIndexOf('</details>'));
    });
  }
});
