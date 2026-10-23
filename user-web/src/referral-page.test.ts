import { readFileSync } from 'node:fs';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { afterEach, describe, expect, it, vi } from 'vitest';
import JoinCodesPanel from './JoinCodesPanel.vue';
import { createUserI18n, userI18nKey } from './i18n';
import ts from 'typescript';

afterEach(() => vi.unstubAllGlobals());
describe('compact referral code page', () => {
  for (const locale of ['en', 'zh-CN']) {
    it(`omits membership, source and extra headings (${locale})`, async () => {
      vi.stubGlobal('navigator', { languages: [locale], language: locale });
      const html = await renderToString(createSSRApp(JoinCodesPanel, { locale: locale === 'en' ? 'en' : 'zh', brandName: 'Aurora' })
        .provide(userI18nKey, createUserI18n()));
      expect(html).toContain(locale === 'en' ? 'Referral codes' : '推荐码');
      expect(html).not.toContain('eyebrow');
      expect(html).not.toContain('<h2');
      expect(html).not.toContain('join-source-code');
      expect(html).not.toContain(locale === 'en' ? 'How you joined' : '您加入此品牌的方式');
    });
  }
  it('requests only own codes and shares referral links, without reading attribution', () => {
    const source = readFileSync(new URL('./JoinCodesPanel.vue', import.meta.url), 'utf8');
    expect(source).not.toMatch(/\.attribution\(/);
    expect(source).not.toContain('MemberAttribution');
    expect(source).toContain("code.kind === 'referral'");
    expect(source).toContain("registrationShareLink(window.location.origin,'referral',code.code)");
    expect(source).not.toContain('agent_code');
    expect(source).not.toContain('code.starts_at');
    expect(source).not.toContain('code.expires_at');
    expect(source).toContain('<RegistrationLink ');
    expect(source).toContain('v-for="code in codes"');
    expect(source).toContain(':url="registerShareLink(code)"');
    expect(source).not.toContain('@click="copyText(code.code)"');
    expect(source).not.toContain('navigator.share');
    expect(source).toContain(':share-title="brandName"');
  });
});

describe('native referral sharing', () => {
  function sharing(navigator: { share?: (data: ShareData) => Promise<void> }) {
    const source = readFileSync(new URL('../../shared/src/RegistrationLink.vue', import.meta.url), 'utf8');
    const fn = source.slice(source.indexOf('async function share()'), source.indexOf('</script>'));
    const code = ts.transpileModule(fn, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
    const notice = {value:''}, sharing = {value:false}, clipboard=vi.fn();
    const run = new Function('navigator','status','sharing','props',`let generation=0; ${code}; return share;`)(navigator,notice,sharing,{disabled:false,shareTitle:'Aurora',url:'https://example.test/register?referral_code=A1B2C3D4E5F607182930ABCD'}) as () => Promise<void>;
    return {run,notice,sharing,clipboard};
  }
  const referral = { code: 'A1B2C3D4E5F607182930ABCD', usable: true };
  it('opens native sharing with the referral URL instead of copying', async () => {
    const share = vi.fn().mockResolvedValue(undefined), test = sharing({ share });
    await test.run();
    expect(share).toHaveBeenCalledWith({ title: 'Aurora', url: `https://example.test/register?referral_code=${referral.code}` });
    expect(test.clipboard).not.toHaveBeenCalled();
    expect(test.sharing.value).toBe(false);
  });
  it('reports unsupported sharing without a clipboard fallback', async () => {
    const test = sharing({}); await test.run();
    expect(test.notice.value).toBe('shareUnsupported');
    expect(test.clipboard).not.toHaveBeenCalled();
  });
  it('treats cancel as cancel, while reporting actual share failures', async () => {
    const share = vi.fn().mockRejectedValue(new DOMException('Cancelled', 'AbortError'));
    const test = sharing({ share }); await test.run();
    expect(test.notice.value).toBe('');
    expect(test.sharing.value).toBe(false);
    share.mockRejectedValue(new DOMException('Denied', 'NotAllowedError')); await test.run();
    expect(test.notice.value).toBe('shareFailed');
    expect(test.clipboard).not.toHaveBeenCalled();
  });
});
