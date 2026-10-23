import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());
describe('mobile header back affordance', () => {
  for (const locale of ['en', 'zh-CN']) for (const path of ['/', '/orders', '/wallet', '/wallet/buckets', '/wallet/ledger', '/account', '/invites', '/help', '/login', '/register']) {
    it(`shows brand at home, parent on non-home pages, and keeps auth chrome (${locale}, ${path})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: Page }] });
      await router.push(path);
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      const auth=['/login','/register'].includes(path);
      const header=html.match(auth?/<header\b[^>]*class="login-entry__topbar"[^>]*>([\s\S]*?)<\/header>/:/<header\b[^>]*class="workspace-topbar"[^>]*>([\s\S]*?)<\/header>/)![1];
      if(auth) expect(header).toContain('class="login-entry__mark"');
      else if(path==='/') { expect(header).toContain('class="workspace-brand"'); expect(header).not.toContain('class="icon-button" aria-label="'+(locale==='en'?'Back':'返回上一页')+'"'); }
      else { expect(header).toContain(locale==='en'?'aria-label="Back"':'aria-label="返回上一页"'); expect(header).not.toContain('class="workspace-brand"'); }
    });
  }
});
