import { publicApiErrorText } from '@lottery/shared'

export function platformErrorMessage(cause: unknown, locale: 'en' | 'zh-CN'): string {
  return publicApiErrorText(cause, locale)
}
