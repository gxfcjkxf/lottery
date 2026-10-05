import { describe, expect, it, vi } from 'vitest'
import { createApiClient } from './api'

const context = {
  brand: {
    id: 'brand-1', code: 'luma', name: 'Luma Play', status: 'active', default_locale: 'zh-CN', timezone: 'Asia/Singapore',
    theme: { primary_color: '#123456', accent_color: '#abcdef' }, config_version: 7,
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
    expect(result.brand).toMatchObject({ id: 'brand-1', code: 'luma', name: 'Luma Play', primary: '#123456', accent: '#abcdef', defaultLanguage: 'zh', version: '7' })
    expect(result.availableLanguages).toEqual(['en', 'zh-CN'])
    expect(result.features?.real_payments).toBe(false)
    expect(result.terms?.service_terms_version).toBe('dev-1')
  })

  it('uses the nested backend error code and message', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, error: { code: 'BRAND_NOT_FOUND', message: 'No brand for this host' } }), { status: 404 }))
    await expect(createApiClient({ fetcher }).getContext()).rejects.toMatchObject({ code: 'BRAND_NOT_FOUND', message: 'No brand for this host', status: 404 })
  })
})
