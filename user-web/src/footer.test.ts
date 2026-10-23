import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());
describe('no redundant footer', () => {
  for (const locale of ['en', 'zh-CN']) for (const path of ['/account', '/results', '/notifications', '/help']) {
    it(`omits the copyright block and normal/checking status (${locale}, ${path})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: Page }] });
      await router.push(path);
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      expect(html).not.toContain('class="page-footer"');
      expect(html).not.toContain('class="sidebar-footer"');
      expect(html).not.toContain('© 2026');
      expect(html).not.toContain('class="connection-pill"');
      expect(html).not.toContain('class="connection-alert"');
    });
  }
});
