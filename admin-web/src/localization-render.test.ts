import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import PresentationFields from './PresentationFields.vue'
import { adminI18nKey, createAdminI18n } from './i18n'
import type { BrandPresentationConfig, BrandPresentationEffective } from './brand-presentation-api'

const effective: BrandPresentationEffective = {
  display_name: '中文品牌', logo_text: '品', logo_url: null, favicon_url: null,
  primary_color: '#27634a', accent_color: '#d7b56d', success_color: '#287b59', warning_color: '#a96d20', danger_color: '#bc4b43',
  font_family: 'system', font_scale: 'standard', radius: 'round', shadow: 'subtle', default_locale: 'zh-CN', available_locales: ['en', 'zh-CN'],
  content: { en: { tagline: 'Unchanged English {name}', announcement: '' }, 'zh-CN': { tagline: '角色权限', announcement: '中文运营文案' } },
}
const config: BrandPresentationConfig = { ...effective, content: structuredClone(effective.content) }

describe('rendered locale and content-field separation', () => {
  it.each(['en', 'zh-CN'] as const)('renders unique accessible names in %s without rewriting business values', async locale => {
    const before = JSON.stringify(config)
    const context = createAdminI18n()
    context.setLocale(locale)
    const app = createSSRApp({ render: () => h(PresentationFields, { config, effective, disabled: false }) })
    app.provide(adminI18nKey, context)
    const html = await renderToString(app)
    for (const label of locale === 'en'
      ? ['English tagline', 'English announcement', 'Chinese tagline', 'Chinese announcement', 'Display name inherit default', 'Primary color inherit default']
      : ['英文标语', '英文公告', '中文标语', '中文公告', '展示名称继承默认', '主色继承默认', '启用English', '启用简体中文']) {
      expect(html.match(new RegExp(`aria-label="${label}"`, 'g'))).toHaveLength(1)
    }
    expect(html).toContain('角色权限')
    expect(html).toContain('中文运营文案')
    expect(html).toContain('Unchanged English {name}')
    expect(html).toMatch(/<option[^>]*value="zh-CN"/)
    expect(JSON.stringify(config)).toBe(before)
  })
  it('keeps business content plain text rather than injecting markup during translation', async () => {
    const malicious = { ...config, display_name: '<img src=x onerror=alert(1)>', content: { en: { tagline: '<script>alert(1)</script>', announcement: '' }, 'zh-CN': { tagline: '中文', announcement: '' } } }
    const context = createAdminI18n()
    context.setLocale('en')
    const app = createSSRApp({ render: () => h(PresentationFields, { config: malicious, effective, disabled: false }) })
    app.provide(adminI18nKey, context)
    const html = await renderToString(app)
    expect(html).not.toContain('<script>')
    expect(html).not.toContain('<img src=x')
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;')
  })
})
