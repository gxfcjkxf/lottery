import { describe, expect, it, vi } from 'vitest'
import { createApiClient } from './api'

const context = {
  brand: {
    id: 'brand-1', code: 'luma', name: 'Harbor 🌅', status: 'active', default_locale: 'zh-CN', timezone: 'Asia/Singapore',
    theme: {
      display_name: 'Harbor 🌅', logo_text: '海', logo_url: 'https://cdn.example/logo.png', favicon_url: '/brand-assets/favicon.webp',
      primary_color: '#123456', accent_color: '#abcdef', success_color: '#123456', warning_color: '#654321', danger_color: '#abcdef',
      font_family: 'serif', font_scale: 'large', radius: 'soft', shadow: 'lifted', default_locale: 'zh-CN', available_locales: ['en', 'zh-CN'],
      content: { en: { tagline: 'Play gently', announcement: 'A note' }, 'zh-CN': { tagline: '轻松参与', announcement: '' } },
    }, config_version: 7,
  },
  available_locales: ['en', 'zh-CN'],
  features: { pwa: true, real_payments: false },
  terms: { privacy_policy_version: 'dev-1', service_terms_version: 'dev-1' },
}

function clientFor(data: unknown) {
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(data), { status: 200 }))
  return { api: createApiClient({ brandCode: 'luma', fetcher }), fetcher }
}

describe('API envelope and tenant context', () => {
  it('unwraps the standard envelope and maps the resolved backend theme', async () => {
    const { api, fetcher } = clientFor({ success: true, data: context })
    const result = await api.getContext()

    expect(fetcher).toHaveBeenCalledWith('/api/v1/b/luma/context', expect.objectContaining({ credentials: 'same-origin' }))
    expect(result.brand).toMatchObject({ id: 'brand-1', code: 'luma', name: 'Harbor 🌅', primary: '#123456', accent: '#abcdef', defaultLanguage: 'zh', version: '7' })
    expect(result.brand).toMatchObject({ logoText: '海', logoUrl: 'https://cdn.example/logo.png', faviconUrl: '/brand-assets/favicon.webp', success: '#123456', warning: '#654321', danger: '#abcdef', fontFamily: 'serif', fontScale: 'large', radius: 'soft', shadow: 'lifted' })
    expect(result.brand.content).toEqual({ en: { tagline: 'Play gently', announcement: 'A note' }, zh: { tagline: '轻松参与', announcement: '' } })
    expect(result.availableLanguages).toEqual(['en', 'zh-CN'])
    expect(result.brand.languages).toEqual(['en', 'zh'])
    expect(result.features?.real_payments).toBe(false)
    expect(result.terms?.service_terms_version).toBe('dev-1')
  })

  it('uses the nested backend theme locales and default locale', async () => {
    const body = structuredClone(context)
    body.brand.theme.default_locale = 'en'
    body.brand.theme.available_locales = ['en']
    const { api } = clientFor({ success: true, data: body })
    const result = await api.getContext()
    expect(result.brand).toMatchObject({ defaultLanguage: 'en', languages: ['en'] })
    expect(result.availableLanguages).toEqual(['en'])
  })

  it('preserves null optional logo and favicon URLs', async () => {
    const body = {
      ...context,
      brand: { ...context.brand, theme: { ...context.brand.theme, logo_url: null, favicon_url: null } },
    }
    const { api } = clientFor({ success: true, data: body })
    const result = await api.getContext()
    expect(result.brand).toMatchObject({ logoUrl: null, faviconUrl: null })
  })

  it('uses the nested backend error code and message', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'BRAND_NOT_FOUND', message: 'No brand for this host' } }), { status: 404 }))
    await expect(createApiClient({ fetcher }).getContext()).rejects.toMatchObject({ code: 'BRAND_NOT_FOUND', message: 'No brand for this host', status: 404 })
  })

  it.each([
    { success: true, data: { brand: { ...context.brand, theme: undefined }, available_locales: context.available_locales, features: context.features, terms: context.terms } },
    { success: true, data: { ...context, brand: { ...context.brand, theme: { ...context.brand.theme, primary_color: 'red' } } } },
    { success: true, data: { ...context, brand: { ...context.brand, theme: { ...context.brand.theme, font_family: 'fantasy' } } } },
    { success: true, data: { ...context, brand: { ...context.brand, theme: { ...context.brand.theme, default_locale: 'fr' } } } },
    { success: true, data: { ...context, brand: { ...context.brand, theme: { ...context.brand.theme, available_locales: ['en', 'fr'] } } } },
    { success: true, data: { ...context, brand: { ...context.brand, theme: { ...context.brand.theme, default_locale: 'zh-CN', available_locales: ['en'] } } } },
  ])('rejects malformed or invalid resolved context %#', async body => {
    const { api } = clientFor(body)
    await expect(api.getContext()).rejects.toThrow('Invalid brand context response')
  })

  it('rejects unsafe non-null asset URLs', async () => {
    const body = structuredClone(context)
    body.brand.theme.logo_url = 'javascript:alert(1)'
    const { api } = clientFor({ success: true, data: body })
    await expect(api.getContext()).rejects.toThrow('unsafe asset URL')
  })

  it.each([
    context,
    { success: true },
    null,
  ])('rejects response bodies outside the standard success envelope %#', async body => {
    const { api } = clientFor(body)
    await expect(api.getContext()).rejects.toThrow('Invalid API response')
  })
})
