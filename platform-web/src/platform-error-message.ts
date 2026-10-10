import { PlatformApiError } from './platform-api'

const messages: Record<string, readonly [string, string]> = {
  "CSRF_REJECTED": [
    "Request origin is not trusted.",
    "请求来源不可信。"
  ],
  "AUTH_INVALID_CREDENTIALS": [
    "Incorrect username or password.",
    "账号或密码不正确。"
  ],
  "AUTH_SESSION_REVOKED": [
    "Your session is unavailable. Please sign in again.",
    "会话不可用，请重新登录。"
  ],
  "ADMIN_ENTRY_MISMATCH": [
    "This account cannot sign in to this administration portal.",
    "账号类型与管理入口不匹配。"
  ],
  "PLATFORM_ADMIN_REQUIRED": [
    "A platform administrator account is required.",
    "需要平台管理员账号。"
  ],
  "PLATFORM_ENTRY_NOT_FOUND": [
    "The platform administration entry is not enabled.",
    "平台管理入口未启用。"
  ],
  "AUTH_BUSY": [
    "Sign-in is busy. Please try again later.",
    "登录繁忙，请稍后重试。"
  ],
  "AUTH_RATE_LIMITED": [
    "Too many attempts. Please try again later.",
    "尝试次数过多，请稍后重试。"
  ],
  "AUTH_UNAVAILABLE": [
    "Authentication is not configured.",
    "认证尚未配置。"
  ],
  "SERVICE_UNAVAILABLE": [
    "The service is temporarily unavailable. Please try again later.",
    "服务暂时不可用，请稍后重试。"
  ],
  "NETWORK_ERROR": [
    "Unable to connect. Please check your connection.",
    "无法连接，请检查网络。"
  ],
  "INVALID_RESPONSE": [
    "Invalid server response; the operation result is unconfirmed.",
    "服务器响应无效，操作结果尚未确认。"
  ],
  "REQUEST_INVALID": [
    "The request is invalid. Please check your input.",
    "请求格式不正确，请检查输入。"
  ],
  "CONTENT_TYPE_INVALID": [
    "The request must use application/json.",
    "请求需要使用 application/json。"
  ],
  "BRAND_REQUIRED": [
    "Please select a valid brand.",
    "请选择有效品牌。"
  ],
  "BRAND_NOT_FOUND": [
    "The brand or administration entry is unavailable.",
    "品牌或管理入口不可用。"
  ],
  "PERMISSION_DENIED": [
    "You do not have permission to perform this action.",
    "无操作权限。"
  ],
  "FORBIDDEN": [
    "You do not have permission to perform this action.",
    "无操作权限。"
  ]
}

export function platformErrorMessage(cause: unknown, locale: 'en' | 'zh-CN'): string {
  const index = locale === 'en' ? 0 : 1
  if (cause instanceof PlatformApiError) {
    const code = cause.network ? 'NETWORK_ERROR' : cause.code
    if (code && messages[code]) return messages[code][index]
    return `${locale === 'en' ? 'Unrecognized API error' : '未识别的接口错误'} (${code || cause.status}).`
  }
  return locale === 'en' ? 'Unexpected error. Please try again.' : '发生意外错误，请重试。'
}
