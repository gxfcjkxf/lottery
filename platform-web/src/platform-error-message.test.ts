import { describe, expect, it } from 'vitest'
import { PlatformApiError } from './platform-api'
import { platformErrorMessage } from './platform-error-message'
import { PlatformAccessApiError } from './access-api'
import { computed, ref } from 'vue'

describe('platform error localization', () => {
  it('localizes assigned-role deletion errors and updates when the language changes', () => {
    const error = new PlatformAccessApiError('raw server reason', 409, 'ROLE_ASSIGNED')
    const locale = ref<'en' | 'zh-CN'>('zh-CN')
    const text = computed(() => platformErrorMessage(error, locale.value))
    expect(text.value).toBe('角色仍被账号使用，请先解除关联后再删除')
    locale.value = 'en'
    expect(text.value).toBe('This role is still assigned to an account. Remove the assignment before deleting it.')
    expect(error.status).toBe(409)
    expect(error.code).toBe('ROLE_ASSIGNED')
  })
  it('uses the error code and selected language instead of server text', () => {
    const error = new PlatformApiError('请求来源不可信', 403, 'CSRF_REJECTED')
    expect(platformErrorMessage(error, 'en')).toBe('Your session has expired. Please sign in again.')
    expect(platformErrorMessage(error, 'zh-CN')).toBe('登录已失效，请重新登录。')
  })
  it('localizes credential and network errors', () => {
    expect(platformErrorMessage(new PlatformApiError('账号或密码不正确', 401, 'AUTH_INVALID_CREDENTIALS'), 'en')).toBe('Incorrect account or password.')
    expect(platformErrorMessage(new PlatformApiError('Failed to fetch', 0, undefined, true), 'zh-CN')).toBe('无法连接，请检查网络。如已提交操作，请先核实结果再重试。')
  })
  it('localizes a replacement notice in both supported languages', () => {
    const error = new PlatformApiError('server text', 401, 'AUTH_SESSION_REPLACED')
    expect(platformErrorMessage(error, 'en')).toBe('A new sign-in has ended this session. Please sign in again.')
    expect(platformErrorMessage(error, 'zh-CN')).toBe('账号有新的登录，当前会话已失效，请重新登录。')
  })
  it('reports unknown failures without displaying codes or server prose', () => {
    expect(platformErrorMessage(new PlatformApiError('后端消息', 409, 'NEW_ERROR'), 'en')).toBe('The data or status has changed. Refresh and review the latest information before continuing.')
    expect(platformErrorMessage(new Error('Internal detail'), 'zh-CN')).toBe('操作未能完成，请检查填写内容和当前状态后再试。')
  })
})
