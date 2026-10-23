import { describe, expect, it } from 'vitest'
import { createCatalogI18n, localizedMessage, nativeLocaleName, supportedLocale, translateMessage } from './localization'

describe('plain-text bilingual messages', () => {
  it('keeps business data and unknown server errors opaque', () => {
    expect(translateMessage('en', '品牌名称')).toBe('品牌名称')
    expect(translateMessage('en', 'REQUEST_FAILED')).toBe('REQUEST_FAILED')
    expect(translateMessage('zh-CN', '登录', 'Sign in')).toBe('登录')
    expect(translateMessage('en', '登录', 'Sign in')).toBe('Sign in')
  })
  it('renders stored notices in the selected language without rewriting their intent', () => {
    const params = { name: '中文品牌 {other}', count: '9007199254740993' }
    const notice = localizedMessage('品牌 {name}：{count} 分', 'Brand {name}: {count} points', params)
    params.count = '0'
    expect(translateMessage('en', notice)).toBe('Brand 中文品牌 {other}: 9007199254740993 points')
    expect(translateMessage('zh-CN', notice)).toBe('品牌 中文品牌 {other}：9007199254740993 分')
    expect(Object.isFrozen(notice)).toBe(true)
  })
  it('does not interpolate inherited properties or rescan substituted values', () => {
    const values = Object.create({ inherited: 'hidden' }) as Record<string, string>
    values.name = '<script>{inherited}</script>'
    expect(translateMessage('en', '{name}/{inherited}/{missing}', undefined, values)).toBe('<script>{inherited}</script>/{inherited}/{missing}')
  })
  it.each(['zh', 'EN', '', null, {}, 'fr'])('rejects unsupported locale %s', value => {
    expect(supportedLocale(value)).toBeNull()
  })
})

describe('catalog i18n provider', () => {
  const catalog = {
    en: { greeting: 'Hello {name}' },
    'zh-CN': { greeting: '你好，{name}' },
  } as const

  it('renders the selected catalog language and parameters', () => {
    const i18n = createCatalogI18n(catalog, 'en')
    expect(i18n.t('greeting', { name: 'Ada' })).toBe('Hello Ada')
    expect(i18n.setLocale('zh-CN')).toBe(true)
    expect(i18n.t('greeting', { name: 'Ada' })).toBe('你好，Ada')
  })

  it('rejects unsupported locale changes and missing catalog keys', () => {
    const i18n = createCatalogI18n(catalog)
    expect(i18n.setLocale('fr')).toBe(false)
    expect(i18n.locale.value).toBe('zh-CN')
    expect(() => i18n.t('missing')).toThrow('Missing zh-CN translation key: missing')
  })

  it('rejects catalogs whose language keys are not identical', () => {
    expect(() => createCatalogI18n({ en: { onlyEnglish: 'x' }, 'zh-CN': { onlyChinese: 'x' } })).toThrow('Translation catalog key mismatch')
  })
})

describe('native locale names', () => {
  it('keeps each language option in its own language', () => {
    expect(nativeLocaleName('en')).toBe('English')
    expect(nativeLocaleName('zh-CN')).toBe('简体中文')
  })
})
