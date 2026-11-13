import { describe, expect, it } from 'vitest'
import { safeBrandAssetUrl } from './presentation'
import { defaultBrand } from './brand'

describe('brand presentation', () => {
  it.each([
    'javascript:alert(1)', 'data:image/png;base64,aaa', 'http://cdn.example/logo.png', '//evil.example/a.png',
    'https://bad host/logo.png', 'https://user:pass@cdn.example/logo.png', 'https://cdn.example:443/logo.png',
    'https://cdn.example/logo.png?token=x', 'https://cdn.example/logo.png#frag', 'https://cdn.example/logo.svg',
    'https://127.0.0.1/logo.png', 'https://127.1/logo.png', 'https://0127.0.0.1/logo.png',
    'https://localhost/logo.png', 'https://cdn.123/logo.png', 'https://cdn.0xdeadbeef/logo.png',
    'HTTPS://cdn.example/logo.png', '/icons/../logo.png', '/icons/%2e%2e/logo.png', '/icons/a b.png',
    '/icons/a\u00a0b.png', '/icons/a\u0085b.png', '/favicon.png', 'assets/logo.png',
  ])('rejects unsafe asset URL %s', value => {
    expect(safeBrandAssetUrl(value)).toBeNull()
  })

  it('accepts HTTPS DNS assets and approved same-origin paths', () => {
    expect(safeBrandAssetUrl('https://cdn.example/logo.PNG')).toBe('https://cdn.example/logo.PNG')
    expect(safeBrandAssetUrl('/icons/logo.png')).toBe('/icons/logo.png')
    expect(safeBrandAssetUrl('/brand-assets/tenant/logo.webp')).toBe('/brand-assets/tenant/logo.webp')
  })

  it('limits asset URIs by Unicode character length', () => {
    expect(safeBrandAssetUrl(`/icons/${'é'.repeat(503)}.png`)).toBeNull()
  })
})

it('replaces stale browser icons and uses the configured name and favicon', async () => {
  const { applyBrowserBranding, applyBrandPresentation } = await import('./presentation')
  const { vi } = await import('vitest')
  let icons: any[] = [{ remove() { icons.splice(icons.indexOf(this), 1) } }]
  const doc = { title: '', createElement() { return { dataset: {}, remove() { icons.splice(icons.indexOf(this), 1) } } }, querySelectorAll() { return [...icons] }, head: { append(link: unknown) { icons.push(link) } } }
  vi.stubGlobal('document', doc)
  try {
    applyBrandPresentation({brand:{...defaultBrand,name:'Brand identity'},paused:false,availableLanguages:['en']})
    expect(doc.title).toBe('Brand identity')
    applyBrowserBranding('Configured platform', null)
    expect(doc.title).toBe('Configured platform')
    expect(icons).toHaveLength(1)
    expect(icons[0].href).toContain('favicon.svg')
    applyBrowserBranding('Other brand', '/brand-assets/brand/icon.png')
    expect(doc.title).toBe('Other brand')
    expect(icons).toHaveLength(1)
    expect(icons[0].href).toBe('/brand-assets/brand/icon.png')
  } finally { vi.unstubAllGlobals() }
})
