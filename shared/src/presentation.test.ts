import { describe, expect, it } from 'vitest'
import { buildBrandCssTokens, safeBrandAssetUrl, safeBrandColor } from './presentation'
import { defaultBrand } from './brand'

describe('brand presentation', () => {
  it('normalizes valid colors and rejects invalid values instead of substituting a fallback', () => {
    expect(safeBrandColor('#ABCDEF')).toBe('#abcdef')
    expect(() => safeBrandColor('red')).toThrow('Invalid brand color')
    expect(() => safeBrandColor(undefined)).toThrow('Invalid brand color')
  })

  it('maps configured values to the shared design system CSS tokens', () => {
    const tokens = buildBrandCssTokens({
      ...defaultBrand,
      primary: '#123456', accent: '#abcdef', success: '#287b59', warning: '#a96d20', danger: '#bc4b43',
      fontFamily: 'mono', fontScale: 'compact', radius: 'square', shadow: 'none',
    })
    expect(tokens).toMatchObject({
      '--primary': '#123456', '--brand-primary': '#123456', '--green': '#287b59', '--primary-dark': '#0e2943',
      '--accent': '#abcdef', '--brand-accent': '#abcdef', '--success': '#287b59', '--warning': '#a96d20', '--danger': '#bc4b43',
      '--font-scale': '14px', '--brand-font-scale-factor': '0.875', '--radius': '2px', '--shadow': 'none',
    })
    expect(tokens['--font-family']).toContain('ui-monospace')
  })

  it('keeps standard text at the base scale and rejects unsupported theme values', () => {
    expect(buildBrandCssTokens({ ...defaultBrand, fontScale: 'standard' })['--brand-font-scale-factor']).toBe('1')
    for (const [key, value] of [
      ['fontFamily', 'system; color: red'], ['fontScale', 'giant'],
      ['radius', '999px'], ['shadow', 'url(javascript:alert(1))'], ['fontScale', undefined],
    ]) {
      expect(() => buildBrandCssTokens({ ...defaultBrand, [key!]: value })).toThrow('Invalid brand presentation')
    }
  })

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

  it('limits asset URIs by UTF-8 byte length', () => {
    expect(safeBrandAssetUrl(`/icons/${'é'.repeat(253)}.png`)).toBeNull()
  })
})

it('replaces stale browser icons and uses the configured name and escaped logo text', async () => {
  const { applyBrowserBranding } = await import('./presentation')
  const { vi } = await import('vitest')
  let icons: any[] = [{ remove() { icons.splice(icons.indexOf(this), 1) } }]
  const doc = { title: '', createElement() { return { dataset: {}, remove() { icons.splice(icons.indexOf(this), 1) } } }, querySelectorAll() { return [...icons] }, head: { append(link: unknown) { icons.push(link) } } }
  vi.stubGlobal('document', doc)
  try {
    applyBrowserBranding('Configured platform', 'Z<&', null, '#059669')
    expect(doc.title).toBe('Configured platform')
    expect(icons).toHaveLength(1)
    expect(decodeURIComponent(icons[0].href)).toContain('Z&lt;&amp;')
    applyBrowserBranding('Other brand', 'B', '/brand-assets/brand/icon.png', '#123456')
    expect(doc.title).toBe('Other brand')
    expect(icons).toHaveLength(1)
    expect(icons[0].href).toBe('/brand-assets/brand/icon.png')
  } finally { vi.unstubAllGlobals() }
})
