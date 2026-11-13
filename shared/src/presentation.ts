import type { PlatformContext } from './brand'

const DNS_HOST_SYNTAX = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$/
const ALLOWED_ASSET_EXTENSIONS = new Set(['.png', '.jpg', '.jpeg', '.webp', '.ico'])

/** Mirror internal/brandskin.ValidAssetURI; hostname validation is syntax-only and never resolves or fetches. */
export function safeBrandAssetUrl(value: unknown): string | null {
  if (typeof value !== 'string' || value.length === 0 || Array.from(value).length > 512 || value.trim() !== value || /[\\%\r\n\t]/.test(value) || /[\p{White_Space}\p{Cc}]/u.test(value) || value.includes('..')) return null
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

/** Brand identity never overrides the shared design system. */
export function applyBrandPresentation(context: PlatformContext): void {
  applyBrowserBranding(context.brand.name, safeBrandAssetUrl(context.brand.faviconUrl))
}

/** Browser tabs use the same configured identity as the visible brand lockup. */
export function applyBrowserBranding(name: string, iconUrl: string | null): void {
  document.title = name
  const link = document.createElement('link')
  link.rel = 'icon'
  link.dataset.brandFavicon = 'true'
  link.href = iconUrl ?? '/favicon.svg'
  link.referrerPolicy = 'no-referrer'
  document.querySelectorAll('link[rel="icon"]').forEach(current => current.remove())
  document.head.append(link)
}
