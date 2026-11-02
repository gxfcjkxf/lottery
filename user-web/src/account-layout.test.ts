import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { createUserI18n } from './i18n';

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
  it('removes member agent route and keeps game-scoped result links', () => {
    expect(readFileSync(new URL('./main.ts', import.meta.url), 'utf8')).not.toContain('path: "/agent"');
    for (const file of ['Page.vue']) {
      const source = readFileSync(new URL(`./${file}`, import.meta.url), 'utf8');
      expect(source).toContain("path:'/results'");
      expect(source).toMatch(/platform_game_id:currentGame.id/);
    }
  });
});
