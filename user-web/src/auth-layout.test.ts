import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createMemoryHistory, createRouter } from 'vue-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Page from './Page.vue';
import { createUserI18n, userI18nKey } from './i18n';

afterEach(() => vi.unstubAllGlobals());
describe('compact authentication layout', () => {
  for (const locale of ['en', 'zh-CN']) for (const path of ['/login', '/register']) {
    it(`has one navigation title and no promotional or unavailable-provider panels (${locale}, ${path})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const paths = ['/', '/login', '/register', '/privacy', '/terms', '/help'];
      const router = createRouter({ history: createMemoryHistory(), routes: paths.map(path => ({ path, component: Page })) });
      await router.push(path);
      const html = await renderToString(createSSRApp(Page).provide(userI18nKey, createUserI18n()).use(router));
      expect((html.match(/<h1(?:\s|>)/g) ?? [])).toHaveLength(1);
      expect(html).not.toContain('class="auth-visual"');
      expect(html).not.toContain('class="form-subtitle"');
      expect(html).not.toContain('class="auth-language-button"');
      expect(html).not.toContain('class="avatar-link"');
      expect(html).toContain('autocomplete="username"');
      expect(html).toContain('novalidate');
      expect(html).toContain('type="password"');
      expect(html).not.toContain('class="auth-switch"');
      expect(html).not.toContain('placeholder=');
      if (path === '/login') {
        expect(html).toContain('auth-login-actions');
        expect(html).toContain('auth-register-button');
        expect(html).toContain(locale === 'en' ? '>Register</a>' : '>注册</a>');
      } else {
        expect(html).toContain('auth-login-button');
        expect(html).toContain(locale === 'en' ? '>Log in</a>' : '>登录</a>');
        expect(html).not.toContain('dev-1');
        expect((html.match(/type="checkbox"/g) ?? [])).toHaveLength(1);
        expect(html).toContain('combined-terms');
        expect(html).toContain('href="/privacy"');
        expect(html).toContain('href="/terms"');
        expect((html.match(/aria-haspopup="dialog"/g) ?? [])).toHaveLength(2);
        expect(html).not.toContain('target="_blank"');
      }
      expect(html).toContain('class="auth-brand-name"');
      expect(html).not.toContain('class="join-code-choice"');
      expect(html).not.toContain(locale === 'en' ? 'Telegram unavailable' : 'Telegram 暂不可用');
    });
  }
});
