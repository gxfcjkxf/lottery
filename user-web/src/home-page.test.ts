import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());

describe('compact game workspace', () => {
  for (const locale of ['en', 'zh-CN']) {
    it(`keeps one game picker and unchanged bottom navigation without empty selection chrome (${locale})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const paths = ['/', '/games', '/help', '/orders', '/wallet', '/results', '/notifications', '/agent', '/invites', '/login', '/account'];
      const router = createRouter({ history: createMemoryHistory(), routes: paths.map(path => ({ path, component: Page })) });
      await router.push('/');
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      expect(html).toContain('class="workspace-topbar"');
      expect(html).toContain('class="workspace-game-trigger"');
      expect(html).toContain('class="mobile-primary-nav"');
      expect(html).not.toContain('href="/rule-preview"');
      expect(html).not.toContain('Rule draft preview');
      expect(html).not.toContain('规则草稿预览');
      expect(html).not.toContain('data-testid="catalog-featured"');
      expect(html).not.toContain('home-responsible-note');
      expect(html).not.toContain('未选择项目');
      expect(html).not.toContain('class="betting-dock panel"');
      expect(html).not.toContain('class="hero"');
      expect(html).not.toContain('class="lower-grid"');
      expect(html).not.toContain('data-testid="home-wallet"');
    });
  }
});
