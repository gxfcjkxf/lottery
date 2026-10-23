import type { BrandTheme, PlatformContext } from './brand'

const HEX_COLOR = /^#[\da-f]{6}$/i
const FONT_FAMILIES = {
  system: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  serif: 'Georgia, "Times New Roman", serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
} as const
const FONT_SCALES = { compact: '14px', standard: '16px' } as const
const RADII = { square: '2px', soft: '8px', round: '16px' } as const
const SHADOWS = {
  none: 'none',
  subtle: '0 18px 55px rgba(28, 54, 44, .08)',
  lifted: '0 18px 55px rgba(28, 54, 44, .16)',
} as const

export function safeBrandColor(value: unknown): string {
  if (typeof value !== 'string' || !HEX_COLOR.test(value)) throw new TypeError('Invalid brand color')
  return value.toLowerCase()
}

const DNS_HOST_SYNTAX = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/
const ALLOWED_ASSET_EXTENSIONS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.ico'])

/** Mirror internal/brandskin.ValidAssetURI; hostname validation is syntax-only and never resolves or fetches. */
export function safeBrandAssetUrl(value: unknown): string | null {
  if (typeof value !== 'string' || value.length === 0 || new TextEncoder().encode(value).length > 512 || value.trim() !== value || /[\\%\r\n\t]/.test(value) || /[\p{White_Space}\p{Cc}]/u.test(value) || value.includes('..')) return null
  if (value.startsWith('https://')) {
    try {
      const parsed = new URL(value)
      const authority = value.slice('https://'.length).split(/[/?#]/, 1)[0] ?? ''
      const host = parsed.hostname.toLowerCase()
      const extension = parsed.pathname.slice(parsed.pathname.lastIndexOf('.')).toLowerCase()
      const hostTail = host.split('.').at(-1) ?? ''
      if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.port || authority.includes(':') || authority.includes('@')) return null
      if (authority.toLowerCase() !== host) return null
      if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(host)) return null
      if (/^(?:[0-9]+|0x[0-9a-f]+)$/.test(hostTail)) return null
      if (!DNS_HOST_SYNTAX.test(host) || !ALLOWED_ASSET_EXTENSIONS.has(extension)) return null
      if (host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.local') || host.endsWith('.internal')) return null
      if (parsed.search || parsed.hash || parsed.href.includes('?') || parsed.href.includes('#')) return null
      return parsed.href
    } catch {
      return null
    }
  }
  if (!value.startsWith('/icons/') && !value.startsWith('/brand-assets/') && !value.startsWith('/api/v1/brand-assets/')) return null
  const extension = value.slice(value.lastIndexOf('.')).toLowerCase()
  if (!ALLOWED_ASSET_EXTENSIONS.has(extension) || /[?#]/.test(value)) return null
  return value
}

function darken(hex: string): string {
  const channels = hex.slice(1).match(/.{2}/g)!.map(channel => Math.max(0, Math.round(parseInt(channel, 16) * 0.78)))
  return `#${channels.map(channel => channel.toString(16).padStart(2, '0')).join('')}`
}

/** Build safe inline CSS variables from an already normalized public context. */
export function buildBrandCssTokens(theme: Pick<BrandTheme, 'primary' | 'accent' | 'success' | 'warning' | 'danger' | 'fontFamily' | 'fontScale' | 'radius' | 'shadow'>): Record<string, string> {
  const primary = safeBrandColor(theme.primary)
  const accent = safeBrandColor(theme.accent)
  const success = safeBrandColor(theme.success)
  const warning = safeBrandColor(theme.warning)
  const danger = safeBrandColor(theme.danger)
  if (!Object.hasOwn(FONT_FAMILIES, theme.fontFamily) || !Object.hasOwn(FONT_SCALES, theme.fontScale) ||
      !Object.hasOwn(RADII, theme.radius) || !Object.hasOwn(SHADOWS, theme.shadow)) throw new TypeError('Invalid brand presentation')
  const family = FONT_FAMILIES[theme.fontFamily]
  const scale = FONT_SCALES[theme.fontScale]
  const scaleFactor = theme.fontScale === 'compact' ? '0.875' : '1'
  const radius = RADII[theme.radius]
  const shadow = SHADOWS[theme.shadow]
  return {
    '--primary': primary,
    '--primary-dark': darken(primary),
    '--primary-light': `color-mix(in srgb, ${primary} 8%, white)`,
    '--primary-ring': `color-mix(in srgb, ${primary} 22%, transparent)`,
    '--green': success,
    '--brand-primary': primary,
    '--accent': accent,
    '--brand-accent': accent,
    '--accent-light': `color-mix(in srgb, ${accent} 8%, white)`,
    '--brand-success': success,
    '--brand-warning': warning,
    '--brand-danger': danger,
    '--success': success,
    '--warning': warning,
    '--danger': danger,
    '--font-family': family,
    '--font-scale': scale,
    '--brand-font-scale-factor': scaleFactor,
    '--radius': radius,
    '--shadow': shadow,
  }
}

export function applyBrandPresentation(context: PlatformContext, root: HTMLElement = document.documentElement): void {
  for (const [token, value] of Object.entries(buildBrandCssTokens(context.brand))) root.style.setProperty(token, value)
  applyBrowserBranding(context.brand.name, context.brand.logoText, safeBrandAssetUrl(context.brand.faviconUrl), context.brand.primary)
}

/** Browser tabs use the same configured identity as the visible brand lockup. */
export function applyBrowserBranding(name: string, logoText: string, iconUrl: string | null, primary: string): void {
  document.title = name
  const escape = (value: string) => value.replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[char]!)
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="${escape(primary)}"/><text x="32" y="34" text-anchor="middle" dominant-baseline="middle" font-family="system-ui,sans-serif" font-size="${Array.from(logoText).length > 2 ? 20 : 34}" font-weight="700" fill="white">${escape(logoText)}</text></svg>`
  const link = document.createElement('link')
  link.rel = 'icon'
  link.dataset.brandFavicon = 'true'
  if (iconUrl) { link.href = iconUrl; link.referrerPolicy = 'no-referrer' }
  else { link.type = 'image/svg+xml'; link.href = `data:image/svg+xml,${encodeURIComponent(svg)}` }
  document.querySelectorAll('link[rel="icon"]').forEach(current => current.remove())
  document.head.append(link)
}
