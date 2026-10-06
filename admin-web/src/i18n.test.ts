import { computed } from 'vue'
import { describe, expect, it } from 'vitest'
import { ADMIN_LOCALE_STORAGE_KEY, createAdminI18n } from './i18n'

describe('admin language preferences', () => {
  it('follows brand defaults until an operator explicitly selects a supported language', () => {
    const context = createAdminI18n()
    const caption = computed(() => context.t('登录', 'Sign in'))
    expect(caption.value).toBe('登录')
    context.configure('en', ['en', 'zh-CN'])
    expect(caption.value).toBe('Sign in')
    context.setLocale('zh-CN')
    context.configure('en', ['en', 'zh-CN'])
    expect(caption.value).toBe('登录')
    context.configure('en', ['en'])
    expect(caption.value).toBe('Sign in')
    expect(context.setLocale('zh-CN')).toBe(false)
    context.configure('en', ['en', 'zh-CN'])
    expect(caption.value).toBe('登录')
  })
  it('retains only the supported locale preference, never account or business records', () => {
    const writes: unknown[] = []
    const context = createAdminI18n({ getItem: () => 'en', setItem: (key, value) => writes.push([key, value]) })
    expect(context.locale.value).toBe('en')
    for (const value of ['EN', 'zh', 'fr', {}, null]) expect(context.setLocale(value)).toBe(false)
    expect(writes).toEqual([])
    expect(context.setLocale('zh-CN')).toBe(true)
    expect(writes).toEqual([[ADMIN_LOCALE_STORAGE_KEY, 'zh-CN']])
    context.resetBrand()
    expect(context.locale.value).toBe('zh-CN')
  })
  it('works with unavailable storage and invalid saved preferences', () => {
    const broken = createAdminI18n({ getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } })
    expect(broken.setLocale('en')).toBe(true)
    expect(broken.locale.value).toBe('en')
    const invalid = createAdminI18n({ getItem: () => 'fr', setItem: () => {} })
    invalid.configure('fr', ['fr', 'en', 'en'])
    expect(invalid.availableLocales.value).toEqual(['en', 'zh-CN'])
    expect(invalid.locale.value).toBe('zh-CN')
  })
  it('reactively translates frozen notices while preserving raw server messages', () => {
    const context = createAdminI18n()
    const notice = context.message('已读取 {count} 条', 'Loaded {count} records', { count: '9007199254740993' })
    const rendered = computed(() => context.t(notice))
    expect(rendered.value).toBe('已读取 9007199254740993 条')
    context.setLocale('en')
    expect(rendered.value).toBe('Loaded 9007199254740993 records')
    expect(context.t('未经翻译的服务端原因')).toBe('未经翻译的服务端原因')
  })
})
