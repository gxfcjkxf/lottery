import type { LocaleCode } from './brand'
import type { TranslationCatalog } from './localization'

// Presentation only: never changes codes, HTTP status, retry decisions or business validation.
export const publicApiErrorCatalog = {
  en: {
    failed: 'The operation could not be completed. Check your input and the current status before trying again.',
    unknown: 'The result could not be confirmed. Check the latest records before retrying; do not submit the same operation again with a new request.',
    network: 'Unable to connect. Check your connection. If you submitted an operation, check its result before retrying.',
    session: 'Your session has expired. Please sign in again.',
    replaced: 'A new sign-in has ended this session. Please sign in again.',
    credentials: 'Incorrect account or password.',
    denied: 'You do not have permission to perform this action.',
    entry: 'This account cannot sign in through this entrance. Use the correct sign-in page.',
    input: 'Check the information you entered and try again.',
    missing: 'This record is no longer available. Refresh the page and check again.',
    changed: 'The data or status has changed. Refresh and review the latest information before continuing.',
    busy: 'This operation is being processed. Please wait and check its result.',
    rate: 'Too many requests. Please wait before trying again.',
    unavailable: 'This service is temporarily unavailable. Please try again later.',
    assigned: 'This role is still assigned to an account. Remove the assignment before deleting it.',
    bootstrap: 'This built-in role cannot be deleted.',
    points: 'Insufficient points. Check your balance before continuing.',
    agentLimit: 'The agent ratio or level exceeds the permitted limit. Check the brand cap, parent ratio and existing subagents.',
    agentCalendar: 'Configure the commission calendar before scheduling an agent mode change.',
    agentModePending: 'This agent line has a scheduled mode change. Editing is available after activation.',
    commissionCorrection: 'This payout was stopped after recalculation. Check the corresponding commission correction record.',
    commissionPoints: 'Insufficient available commission points. The correction is paused; check the commission balance and frozen points.',
    limit: 'The amount or quantity exceeds the permitted limit. Reduce it before continuing.',
    legacyPeriod: 'This period contains historical tickets. New sales begin with the next period.',
    closed: 'This period is closed. Select the current open period before betting.',
    paused: 'This brand is not currently open for this operation.',
    duplicate: 'This account or name already exists. Check it before continuing.',
    captcha: 'The verification code is invalid or expired. Refresh it and try again.',
    password: 'The password does not meet the requirements. Check it and try again.',
    ineligible: 'Withdrawal requirements have not been met. Check your withdrawal eligibility.',
    activeWithdrawal: 'A withdrawal is already in progress. Check that request before submitting another.',
    locked: 'This information cannot be changed here.',
    gamePending: 'This game has unpublished periods or unsettled orders. Complete them before deleting it.',
    inUse: 'This item is still in use. Remove its associations before deleting it.',
    reconcile: 'The points records need to be checked. Please contact an administrator before continuing.',
    rules: 'The game configuration is not ready. This operation is currently unavailable.',
    terms: 'Please read and accept the terms before continuing.',
    join: 'Join this brand before continuing.',
    accountChanged: 'The signed-in account has changed. Review the account and operation before continuing.',
    tooLarge: 'Too many records. Narrow the date range or filters and try again.',
    disabled: 'This account is unavailable. Please contact an administrator.',
    readFailed: 'Unable to load the latest data. Please refresh again later.',
  },
  'zh-CN': {
    failed: '操作未能完成，请检查填写内容和当前状态后再试。',
    unknown: '暂时无法确认操作结果，请先查看最新记录；重试时请使用原操作，不要重复提交新请求。',
    network: '无法连接，请检查网络。如已提交操作，请先核实结果再重试。',
    session: '登录已失效，请重新登录。',
    replaced: '账号有新的登录，当前会话已失效，请重新登录。',
    credentials: '账号或密码不正确。',
    denied: '你没有执行此操作的权限。',
    entry: '此账号不能从当前入口登录，请使用对应的登录页面。',
    input: '请检查填写的信息后重试。',
    missing: '该记录已不存在或当前不可用，请刷新后查看。',
    changed: '数据或状态已变化，请刷新并核对最新信息后再操作。',
    busy: '此操作正在处理中，请稍候查看结果。',
    rate: '操作过于频繁，请稍后再试。',
    unavailable: '服务暂时不可用，请稍后再试。',
    assigned: '角色仍被账号使用，请先解除关联后再删除',
    bootstrap: '系统内置角色不能删除。',
    points: '积分不足，请核对积分余额后再操作。',
    agentLimit: '代理比例或层级超出上限，请检查品牌比例上限、上级比例和已有下级。',
    agentCalendar: '请先配置佣金日历，再预约代理模式切换。',
    agentModePending: '整条代理线已有待生效模式，请在生效后再修改。',
    commissionCorrection: '该发放记录因重新核算已停止，请查看对应的佣金更正记录。',
    commissionPoints: '可用佣金积分不足，更正已暂停。请核对佣金余额及冻结状态。',
    limit: '金额或数量超出允许范围，请减少后再操作。',
    legacyPeriod: '本期已有旧协议注单，新投注将从下一期开始。',
    closed: '本期已封盘，请选择当前可投注的期号。',
    paused: '当前品牌暂未开放此操作。',
    duplicate: '该账号或名称已存在，请核对后再操作。',
    captcha: '验证码不正确或已过期，请刷新后重新填写。',
    password: '密码不符合要求，请检查后重新填写。',
    ineligible: '尚未满足提现条件，请查看提现资格。',
    activeWithdrawal: '已有提现申请正在处理中，请先查看该申请。',
    locked: '此信息不能在这里修改。',
    gamePending: '有未开奖期次或未结算订单，请完成后再删除彩种。',
    inUse: '此项目仍在使用中，请先解除关联后再删除。',
    reconcile: '积分记录需要核对，请联系管理员处理后再操作。',
    rules: '彩种配置尚未就绪，暂时无法进行此操作。',
    terms: '请先阅读并同意相关条款。',
    join: '请先加入当前品牌后再操作。',
    accountChanged: '当前登录账号已变化，请核对账号及操作内容后再继续。',
    tooLarge: '记录数量过多，请缩小日期范围或筛选条件后重试。',
    disabled: '此账号当前不可用，请联系管理员。',
    readFailed: '暂时无法读取最新数据，请稍后刷新。',
  },
} satisfies TranslationCatalog

type ErrorKey = keyof typeof publicApiErrorCatalog.en
const codes: Record<string, ErrorKey> = {
  AGENT_COMMISSION_CALENDAR_REQUIRED: 'agentCalendar', AGENT_MODE_PENDING: 'agentModePending',
  ROLE_ASSIGNED: 'assigned', ROLE_BOOTSTRAP: 'bootstrap',
  AUTH_INVALID_CREDENTIALS: 'credentials', AUTH_SESSION_REPLACED: 'replaced',
  AUTH_SESSION_REVOKED: 'session', AUTH_ACTOR_CONTEXT_CHANGED: 'accountChanged',
  BET_CONFIRMATION_ACCOUNT_CHANGED: 'accountChanged', WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED: 'accountChanged',
  ADMIN_ENTRY_MISMATCH: 'entry', PLATFORM_ENTRY_NOT_FOUND: 'entry', PLATFORM_ADMIN_REQUIRED: 'entry',
  CSRF_REJECTED: 'session', AUTH_RATE_LIMITED: 'rate',
  NETWORK_ERROR: 'network', INVALID_RESPONSE: 'unknown', INTERNAL_ERROR: 'unknown',
  AUTH_BUSY: 'busy', CAPTCHA_INVALID: 'captcha', PASSWORD_INVALID: 'password',
  COMMISSION_PAYMENT_CORRECTION_REQUIRED: 'commissionCorrection', COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT: 'commissionPoints',
  POINTS_INSUFFICIENT: 'points', INSUFFICIENT_POINTS: 'points', COMMISSION_ADJUSTMENT_INSUFFICIENT: 'points',
  BET_LEGACY_PERIOD: 'legacyPeriod', BET_PERIOD_CLOSED: 'closed', PERIOD_CLOSED: 'closed',
  BRAND_PAUSED: 'paused', BRAND_DISABLED: 'paused',
  AGENT_LIMIT_CONFLICT: 'agentLimit',
  IDENTITY_EXISTS: 'duplicate', BRAND_CODE_CONFLICT: 'duplicate',
  WITHDRAWAL_INELIGIBLE: 'ineligible', WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED: 'ineligible',
  WITHDRAWAL_ACTIVE_ORDER: 'activeWithdrawal', POINTS_RECONCILIATION_REQUIRED: 'reconcile',
  PROFILE_IMMUTABLE: 'locked', JOIN_ATTRIBUTION_FIXED: 'locked', TELEGRAM_BINDING_IMMUTABLE: 'locked',
  PLATFORM_GAME_MODEL_LOCKED: 'locked', PLATFORM_GAME_IN_USE: 'gamePending',
  MARK_SIX_ATTRIBUTES_MISSING: 'rules', RULE_VALIDATION_REQUIRED: 'rules',
  TERMS_REQUIRED: 'terms', BRAND_JOIN_REQUIRED: 'join', MEMBER_DISABLED: 'disabled',
}

export function publicApiErrorKey(cause: unknown): ErrorKey {
  const error = cause && typeof cause === 'object' ? cause as { code?: unknown; status?: unknown; network?: unknown } : {}
  const code = typeof error.code === 'string' ? error.code.toUpperCase() : ''
  if (codes[code]) return codes[code]
  if (error.network === true) return 'network'
  if (/_VERSION_CONFLICT$|_STATE_CONFLICT$|_EVIDENCE_CONFLICT$|^VERSION_CONFLICT$/.test(code)) return 'changed'
  if (/_INPUT_INVALID$|_INVALID$|^REQUEST_INVALID$/.test(code)) return 'input'
  if (/_TOO_LARGE$/.test(code)) return 'tooLarge'
  if (/_LIMIT$|_LIMIT_EXCEEDED$|_LIMIT_CONFLICT$|_OVERFLOW$/.test(code)) return 'limit'
  if (/_NOT_FOUND$|^NOT_FOUND$/.test(code)) return 'missing'
  if (/_DENIED$|_REQUIRED$/.test(code)) return 'denied'
  if (/_BUSY$/.test(code)) return 'busy'
  if (/_UNAVAILABLE$|_NOT_READY$|_DISABLED$/.test(code)) return 'unavailable'
  if (error.status === 401) return 'session'
  if (error.status === 403) return 'denied'
  if (error.status === 404) return 'missing'
  if (error.status === 409) return 'changed'
  if (error.status === 429) return 'rate'
  if (error.status === 0 || typeof error.status === 'number' && error.status >= 500) return 'unknown'
  return 'failed'
}

export function publicApiErrorText(cause: unknown, locale: LocaleCode): string {
  return publicApiErrorCatalog[locale][publicApiErrorKey(cause)]
}
