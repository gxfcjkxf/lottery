import { describe, expect, it, vi } from 'vitest'
import { createApiClient } from './api'

const context = {
  brand: {
    id: 'brand-1', code: 'luma', name: 'Luma Play', status: 'active', default_locale: 'zh-CN', timezone: 'Asia/Singapore',
    theme: {
      display_name: 'Harbor 🌅', logo_text: '海', logo_url: 'https://cdn.example/logo.png', favicon_url: '/brand-assets/favicon.webp',
      primary_color: '#123456', accent_color: '#abcdef', success_color: '#123456', warning_color: '#654321', danger_color: '#abcdef',
      font_family: 'serif', font_scale: 'large', radius: 'soft', shadow: 'lifted',
      content: { en: { tagline: 'Play gently', announcement: 'A note' }, 'zh-CN': { tagline: '轻松参与', announcement: '' } },
    }, config_version: 7,
  },
  available_locales: ['en', 'zh-CN'],
  features: { pwa: true, real_payments: false },
  terms: { privacy_policy_version: 'dev-1', service_terms_version: 'dev-1' },
}

describe('API envelope and tenant context', () => {
  it('unwraps the backend envelope and normalizes snake_case brand fields', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: context }), { status: 200 }))
    const api = createApiClient({ brandCode: 'luma', fetcher })
    const result = await api.getContext()

    expect(fetcher).toHaveBeenCalledWith('/api/v1/b/luma/context', expect.objectContaining({ credentials: 'same-origin' }))
    expect(result.brand).toMatchObject({ id: 'brand-1', code: 'luma', name: 'Harbor 🌅', primary: '#123456', accent: '#abcdef', defaultLanguage: 'zh', version: '7' })
    expect(result.brand).toMatchObject({ logoText: '海', logoUrl: 'https://cdn.example/logo.png', faviconUrl: '/brand-assets/favicon.webp', success: '#123456', warning: '#654321', danger: '#abcdef', fontFamily: 'serif', fontScale: 'large', radius: 'soft', shadow: 'lifted' })
    expect(result.brand.content).toEqual({ en: { tagline: 'Play gently', announcement: 'A note' }, zh: { tagline: '轻松参与', announcement: '' } })
    expect(result.availableLanguages).toEqual(['en', 'zh-CN'])
    expect(result.features?.real_payments).toBe(false)
    expect(result.terms?.service_terms_version).toBe('dev-1')
  })

  it('uses the nested backend error code and message', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'BRAND_NOT_FOUND', message: 'No brand for this host' } }), { status: 404 }))
    await expect(createApiClient({ fetcher }).getContext()).rejects.toMatchObject({ code: 'BRAND_NOT_FOUND', message: 'No brand for this host', status: 404 })
  })

  it('filters unsupported and duplicate locales and falls back to a supported default', async () => {
    const body = structuredClone(context)
    body.brand.default_locale = 'fr'
    body.available_locales = ['zh-CN', 'unknown', 'zh-CN', 'en', 'fr']
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: body }), { status: 200 }))
    const result = await createApiClient({ fetcher }).getContext()
    expect(result.availableLanguages).toEqual(['zh-CN', 'en'])
    expect(result.brand.languages).toEqual(['zh', 'en'])
    expect(result.brand.defaultLanguage).toBe('en')
  })

  it('uses bounded defaults for invalid theme values and rejects unsafe asset URLs', async () => {
    const body = structuredClone(context)
    body.brand.theme = {
      ...body.brand.theme,
      primary_color: 'red; background:url(javascript:alert(1))',
      font_family: '"; background:url(x)', font_scale: 'huge', radius: '999px', shadow: 'url(javascript:1)',
      logo_url: 'javascript:alert(1)', favicon_url: 'data:image/svg+xml,<svg/>',
    }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: body }), { status: 200 }))
    const result = await createApiClient({ fetcher }).getContext()
    expect(result.brand).toMatchObject({ primary: '#20594c', fontFamily: 'system', fontScale: 'standard', radius: 'round', shadow: 'subtle', logoUrl: null, faviconUrl: null })
  })

  it('uses the effective display name rune and maps effective theme locale fields', async () => {
    const body = structuredClone(context)
    const theme = body.brand.theme as Record<string, unknown>
    theme.display_name = '彩票平台'
    theme.logo_text = ''
    theme.default_locale = 'en'
    theme.available_locales = ['en']
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: true, data: body }), { status: 200 }))
    const result = await createApiClient({ fetcher }).getContext()
    expect(result.brand).toMatchObject({ name: '彩票平台', logoText: '彩', defaultLanguage: 'en', languages: ['en'] })
    expect(result.availableLanguages).toEqual(['en'])
  })
})
