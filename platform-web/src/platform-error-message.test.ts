import { describe, expect, it } from 'vitest'
import { PlatformApiError } from './platform-api'
import { platformErrorMessage } from './platform-error-message'

describe('platform error localization', () => {
  it('uses the error code and selected language instead of server text', () => {
    const error = new PlatformApiError('请求来源不可信', 403, 'CSRF_REJECTED')
    expect(platformErrorMessage(error, 'en')).toBe('Request origin is not trusted.')
    expect(platformErrorMessage(error, 'zh-CN')).toBe('请求来源不可信。')
  })
  it('localizes credential and network errors', () => {
    expect(platformErrorMessage(new PlatformApiError('账号或密码不正确', 401, 'AUTH_INVALID_CREDENTIALS'), 'en')).toBe('Incorrect username or password.')
    expect(platformErrorMessage(new PlatformApiError('Failed to fetch', 0, undefined, true), 'zh-CN')).toBe('无法连接，请检查网络。')
  })
  it('reports unknown codes explicitly without displaying server prose', () => {
    expect(platformErrorMessage(new PlatformApiError('后端消息', 409, 'NEW_ERROR'), 'en')).toBe('Unrecognized API error (NEW_ERROR).')
    expect(platformErrorMessage(new Error('Internal detail'), 'zh-CN')).toBe('发生意外错误，请重试。')
  })
})
