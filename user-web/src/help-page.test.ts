import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';

afterEach(() => vi.unstubAllGlobals());

describe('help page number-model guidance', () => {
  for (const locale of ['en', 'zh']) {
    it(`explains configured digit repetition without imposing unique numbers (${locale})`, async () => {
      vi.stubGlobal('localStorage', { getItem: () => locale });
      const paths = ['/', '/help', '/orders', '/wallet', '/results', '/notifications', '/agent', '/invites', '/login'];
      const router = createRouter({ history: createMemoryHistory(), routes: paths.map(path => ({ path, component: Page })) });
      await router.push('/help');
      const html = await renderToString(createSSRApp(Page).use(router));
      expect(html).toContain(locale === 'en' ? 'digit-position games' : '数字位玩法');
      expect(html).toContain(locale === 'en' ? 'repeated digits such as 111' : '111 这样的重复数字');
      expect(html).toContain(locale === 'en' ? 'server quote' : '服务器报价');
      expect(html).not.toContain(locale === 'en' ? 'Choose at least the required number of unique numbers.' : '选择不少于规定数量的不同号码。');
    });
  }
});
