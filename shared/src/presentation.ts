import type { BrandTheme, PlatformContext } from './brand'

const HEX_COLOR = /^#[\da-f]{6}$/i
const FONT_FAMILIES = {
  system: 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  serif: 'Georgia, "Times New Roman", serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
} as const
const FONT_SCALES = { compact: '14px', standard: '16px', large: '18px' } as const
const RADII = { square: '2px', soft: '8px', round: '16px' } as const
const SHADOWS = {
  none: 'none',
  subtle: '0 18px 55px rgba(28, 54, 44, .08)',
  lifted: '0 18px 55px rgba(28, 54, 44, .16)',
} as const

export function safeBrandColor(value: unknown, fallback: string): string {
  return typeof value === 'string' && HEX_COLOR.test(value) ? value.toLowerCase() : fallback
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
  if (!value.startsWith('/icons/') && !value.startsWith('/brand-assets/')) return null
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
  const primary = safeBrandColor(theme.primary, '#20594c')
  const accent = safeBrandColor(theme.accent, '#d9ef9b')
  const success = safeBrandColor(theme.success, '#287b59')
  const warning = safeBrandColor(theme.warning, '#a96d20')
  const danger = safeBrandColor(theme.danger, '#bc4b43')
  const family = Object.hasOwn(FONT_FAMILIES, theme.fontFamily) ? FONT_FAMILIES[theme.fontFamily as keyof typeof FONT_FAMILIES] : FONT_FAMILIES.system
  const scale = theme.fontScale && Object.hasOwn(FONT_SCALES, theme.fontScale) ? FONT_SCALES[theme.fontScale as keyof typeof FONT_SCALES] : FONT_SCALES.standard
  const scaleFactor = theme.fontScale === 'compact' ? '0.875' : theme.fontScale === 'large' ? '1.125' : '1'
  const radius = Object.hasOwn(RADII, theme.radius) ? RADII[theme.radius as keyof typeof RADII] : RADII.round
  const shadow = Object.hasOwn(SHADOWS, theme.shadow) ? SHADOWS[theme.shadow as keyof typeof SHADOWS] : SHADOWS.subtle
  return {
    '--primary': primary,
    '--primary-dark': darken(primary),
    '--green': success,
    '--brand-primary': primary,
    '--accent': accent,
    '--brand-accent': accent,
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
  const favicon = safeBrandAssetUrl(context.brand.faviconUrl)
  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"][data-brand-favicon]')
  if (favicon) {
    if (!link) {
      link = document.createElement('link')
      link.rel = 'icon'
      link.dataset.brandFavicon = 'true'
      link.crossOrigin = 'anonymous'
      link.referrerPolicy = 'no-referrer'
      document.head.append(link)
    }
    link.href = favicon
  } else if (link) {
    link.remove()
  }
}
