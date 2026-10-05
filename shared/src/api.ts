import { defaultBrand, type PlatformContext } from './brand'

export class ApiError extends Error {
  constructor(message: string, readonly status?: number, readonly code?: string) {
    super(message)
    this.name = 'ApiError'
  }
}

export interface ApiClientOptions {
  baseUrl?: string
  brandCode?: string
  fetcher?: typeof fetch
}

interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: { code?: string; message?: string; details?: unknown }
  message?: string
  code?: string
}

interface BackendContext {
  brand: {
    id: string
    code: string
    name: string
    status: string
    default_locale: string
    timezone: string
    theme?: { primary_color?: string; accent_color?: string; [key: string]: unknown }
    config_version: number
  }
  available_locales: string[]
  features: { pwa: boolean; real_payments: boolean }
  terms: { privacy_policy_version: string; service_terms_version: string }
}

function isEnvelope<T>(value: unknown): value is ApiEnvelope<T> {
  return typeof value === 'object' && value !== null && 'success' in value
}

function toPlatformContext(value: BackendContext): PlatformContext {
  const theme = value.brand.theme ?? {}
  const defaultLanguage = value.brand.default_locale === 'zh-CN' ? 'zh' : value.brand.default_locale === 'zh' ? 'zh' : 'en'
  return {
    brand: {
      ...defaultBrand,
      id: value.brand.id,
      code: value.brand.code,
      name: value.brand.name,
      primary: theme.primary_color ?? defaultBrand.primary,
      accent: theme.accent_color ?? defaultBrand.accent,
      defaultLanguage,
      languages: value.available_locales.map(locale => locale === 'zh-CN' ? 'zh' : locale === 'zh' ? 'zh' : 'en'),
      version: String(value.brand.config_version),
      status: value.brand.status,
      timezone: value.brand.timezone,
    },
    paused: value.brand.status === 'paused',
    availableLanguages: value.available_locales,
    features: value.features,
    terms: value.terms,
    configVersion: value.brand.config_version,
  }
}

export function createApiClient({ baseUrl = '', brandCode, fetcher = fetch }: ApiClientOptions = {}) {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    let response: Response
    try {
      response = await fetcher(`${baseUrl}${path}`, {
        ...init,
        headers: { Accept: 'application/json', ...init.headers },
        credentials: 'same-origin',
      })
    } catch (error) {
      throw new ApiError(error instanceof Error ? error.message : 'Network unavailable')
    }
    const body: unknown = await response.json().catch(() => undefined)
    if (isEnvelope<T>(body)) {
      if (!response.ok || !body.success) {
        throw new ApiError(body.error?.message ?? body.message ?? `Request failed (${response.status})`, response.status, body.error?.code ?? body.code)
      }
      return body.data as T
    }
    if (!response.ok) throw new ApiError(`Request failed (${response.status})`, response.status)
    return body as T
  }

  return {
    request,
    getContext: async () => {
      const path = brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}/context` : '/api/v1/context'
      return toPlatformContext(await request<BackendContext>(path))
    },
  }
}

export type ApiClient = ReturnType<typeof createApiClient>
