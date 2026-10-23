import { readApiEnvelope } from './api-response'

import type { RuleModel, RuleDefinition } from './rules'

export interface PlatformGame {
  id: string; code: string; name: string; model: RuleModel; timezone: string; version: number; connected: boolean; status: 'active' | 'offline' | 'deleted'; deletable: boolean; brand_enabled: boolean; brand_version: number; rule_published: boolean; sales_status: 'ready' | 'disabled' | 'outdated' | 'unconfigured'
}
export interface PlatformPeriod {
 attributes?: import('./mark-six').MarkSixAttributes|null
  id: string; game_id: string; period_no: string; bet_start_at: string; bet_end_at: string; draw_at: string
  result: { regular: number[]; special: number[]; digits: number[] } | null; published_at: string | null
}
export interface PlatformGameDetail {
  rule_protocol: 'generic' | 'configured'
  mark_six_configured?: boolean
  game: PlatformGame
  plays: Array<{ id: string; code: string; name: string; definition: RuleDefinition; definition_hash: string }>
  periods: PlatformPeriod[]
}
export class PlatformLotteryError extends Error {
  constructor(public code: string, public status: number) { super(code) }
}
export function createPlatformLotteryClient(surface: 'platform' | 'admin' | 'user', brandId: () => string, fetcher: typeof fetch = fetch, brandCode?: string) {
  async function request<T>(path: string, body?: unknown, key?: string): Promise<T> {
    const headers = new Headers({ Accept: 'application/json' })
    if (surface === 'admin') headers.set('X-Brand-ID', brandId())
    if (body !== undefined) { headers.set('Content-Type', 'application/json'); headers.set('Idempotency-Key', key!) }
    let response: Response
    const base = surface === 'user' ? `/api/v1${brandCode ? `/b/${encodeURIComponent(brandCode)}` : ''}` : `/api/v1/${surface}`
    try { response = await fetcher(`${base}/platform-games${path}`, { credentials: 'same-origin', method: body === undefined ? 'GET' : 'POST', headers, ...(body === undefined ? {} : { body: JSON.stringify(body) }) }) }
    catch { throw new PlatformLotteryError('NETWORK_ERROR', 0) }
    const value = await readApiEnvelope<T>(response, (_message,status,code)=>new PlatformLotteryError(code,status))
  return value.data
  }
  return {
    drawSources:(id:string)=>request<import('./automation').DrawSourceList>(`/${encodeURIComponent(id)}/draw-sources`),
    automation:(id:string)=>request<import('./automation').AutomationConfig>(`/${encodeURIComponent(id)}/automation`),
    list: (offset = 0) => request<{ games: PlatformGame[]; limit: number; offset: number }>(`?limit=50&offset=${offset}`),
    detail: (id: string, offset = 0) => request<PlatformGameDetail>(`/${encodeURIComponent(id)}?limit=50&offset=${offset}`),
    write: (path: string, body: unknown, key: string) => request<unknown>(path, body, key),
  }
}

export const platformLotteryCatalog = {
 en: {
  backToGames: 'Back to game list',
 editName: 'Rename game', ruleUnpublished: 'Rules not published', rulePublished: 'Rules published', salesReady: 'Sales enabled', salesUnconfigured: 'Sales not configured', salesDisabled: 'Sales disabled', salesOutdated: 'Sales rules need updating', brandNotConnected: 'Brand not connected',

 drawConfiguration:'Draw configuration', drawSources:'Data sources', manualResults:'Manual results',
 gameActionPrompt: '{action} “{name}”?',
 actions: 'Actions', status: 'Status', active: 'Listed', offline: 'Offline', disabled: 'Disabled for brand', enabled: 'Enabled for brand', allStates: 'All statuses', lifecycleFilter: 'Game status', takeOffline: 'Take offline', relist: 'Relist', deleteGame: 'Delete game', disableGame: 'Disable for brand', enableGame: 'Enable for brand', lifecycleConfirm: 'Confirm game change', confirmAction: 'Confirm', cancelAction: 'Cancel', offlineHint: 'Hide this game from player listings and stop new bets and new automatic periods. Existing periods and orders continue to draw and settle; history and wallet entries stay available.', brandDisableHint: 'Hide this game and stop new bets for this brand only. Existing orders continue to settle and history and wallet entries are retained.', enableHint: 'Restore catalogue availability. Existing sales and automatic-draw settings still apply.', cannotDelete: 'This game has unpublished periods or unsettled orders.', 'error.PLATFORM_GAME_IN_USE': 'This game has unpublished periods or unsettled orders.',

 automation:'Automatic draws',
 canada:'Canada betting',
  markSix: 'Mark Six',
  playSelectionTitle: 'Selection and stake', playSelectionHelp: 'Choose how many regular and special numbers a player selects per combination. The game’s number model stays unchanged.', playDigitsSelection: 'Select one digit (0–9) at each of {count} ordered positions.', playRegularCount: 'Regular numbers per combination', playSpecialCount: 'Special numbers per combination', playUnit: 'Points per combination',
  playTiersTitle: 'Winning conditions and payouts', playTiersHelp: "The initial tier requires all selections to match. Check conditions and odds. Payout = points per combination × odds × stake multiplier.", playTierNumber: 'Prize tier {count}', playRemoveTier: 'Remove tier', playAddTier: 'Add prize tier', playOdds: 'Payout multiplier (odds)', playTierCap: 'Tier payout cap (points)', playNoCap: 'No cap', playTierPayout: 'Tier payout method', playExclusive: 'Exclusive: use the highest payout', playAdditive: 'Additive: add this tier’s payout', playConditionLogic: 'Match conditions', playAllConditions: 'All conditions must match', playAnyCondition: 'Any condition may match', playConditionField: 'What to compare', playDrawScope: 'Draw numbers to use', playScopeAll: 'All draw numbers', playScopeDigits: 'Draw digits', playComparison: 'Comparison', playEquals: 'Equals', playBetween: 'Within range (inclusive)', playValue: 'Value', playMin: 'Minimum', playMax: 'Maximum', playRemoveCondition: 'Remove condition', playAddCondition: 'Add condition', playMixedPolicy: 'When exclusive and additive tiers both match', playChoosePolicy: 'Choose a payout policy', playMixedMax: 'Pay only the highest single tier', playMixedAdd: 'Highest exclusive payout plus all additive payouts',
  playLimitsTitle: 'Limits and rounding', playLimitsHelp: "Combination limits apply to expanded selections. Amount caps are optional; final points are rounded half up.", playOrderCap: 'Total payout cap (points)', playMaxCombinations: 'Maximum combinations', playMaxMultiplier: 'Maximum stake multiplier', playMaxBet: 'Maximum stake per order (points)', playRounding: 'When to round points', playRoundOrder: 'After totaling the order', playRoundLine: 'After each combination', playRoundTier: 'After each prize tier',
  playHelpTitle: 'Configuration guide', playHelpConditions: 'Match counts compare the player’s selection with the draw. Other comparisons use the draw’s sum, odd/even counts or span. “All numbers identical” uses 1 for yes and 0 for no. Span is the largest minus the smallest number; longest consecutive sequence is a count. This form configures number selections.', playHelpOdds: 'Example: 2 points per combination, payout multiplier 3, stake multiplier 1 gives a 6-point payout for one matching combination, before caps and rounding. The initial multiplier 1 is an example, not an approved payout rate.', playHelpSave: "Rules cannot be edited after saving. Mark Six also requires a published sales configuration before betting opens.", playFormInvalid: 'Check the selection counts, conditions, odds, integer point amounts and limits. Choose a payout policy when combining exclusive and additive tiers.',
  'playField.position_match': 'Matching digit positions', 'playField.regular_match': 'Matching regular numbers', 'playField.special_match': 'Matching special numbers', 'playField.draw_sum': 'Sum of draw numbers', 'playField.draw_odd_count': 'Odd number count', 'playField.draw_even_count': 'Even number count', 'playField.draw_unique_count': 'Distinct number count', 'playField.draw_all_same': 'All numbers identical (1=yes, 0=no)', 'playField.draw_span': 'Largest minus smallest number', 'playField.draw_consecutive': 'Longest consecutive sequence',
  calendarPreviousMonth: 'Previous month', calendarNextMonth: 'Next month', calendarToday: 'Today', timeHour: 'Hour', timeMinute: 'Minute', timeSecond: 'Second', timeMeridiem: 'AM/PM', timeAM: 'AM', timePM: 'PM',
  editModel: 'Edit number model', modelLocked: 'Games with plays or periods cannot overwrite their number model.', minimum: 'Minimum number', maximum: 'Maximum number', numberPool: 'Number pool', regularPool: 'Regular number pool', specialPool: 'Special number pool', poolMode: 'Pool definition', range: 'Number range', numberList: 'Numbers (comma-separated)',
  'error.MARK_SIX_ATTRIBUTES_MISSING': 'Publish the annual number attributes for this draw date before creating a period.',
  'error.PLATFORM_GAME_MODEL_LOCKED': 'The game already has plays or periods. Its number model cannot be overwritten.',
  'error.PLATFORM_GAME_VERSION_CONFLICT': 'The game was updated. Reopen its model before editing.',
  ownership: 'Owner', platformOwner: 'Platform', loading: 'Loading…',
  modelDigits: 'Ordered digits (0–9)', modelXY: 'Regular and special numbers', modelMN: 'Choose numbers from a pool',
  title: 'Platform games', name: 'Name', code: 'Code', timezone: 'Time zone', periods: 'Periods', plays: 'Play rules', model: 'Number model', connect: 'Connect', connected: 'Connected', create: 'Create game', close: 'Close', refresh: 'Refresh', previous: 'Previous page', next: 'Next page', empty: 'No records', reason: 'Reason', save: 'Save', retry: 'Retry original request', readonly: 'Number models, periods and results are managed by the platform and shared by connected brands. Brands configure their own Mark Six sales and limits.',
  type: 'Model type', digits: 'Digits', regularCount: 'Regular count', specialCount: 'Special count', pool: 'Pool maximum', repeat: 'Allow repeats', ordered: 'Ordered', result: 'Result', periodNo: 'Period number', start: 'Betting opens', end: 'Betting closes', drawAt: 'Draw time', date: 'Date', time: 'Time', pending: 'Not published', publish: 'Publish result', createPeriod: 'Create period', createPlay: 'Create play rule', definition: 'Rule definition', numbers: 'Numbers (comma-separated)', regular: 'Regular numbers', special: 'Special numbers', awaiting: 'Request result unknown. Retry the same request.', success: 'Saved',
  'error.NETWORK_ERROR': 'Connection failed. Retry the original request.', 'error.INVALID_RESPONSE': 'Invalid server response.', 'error.PLATFORM_GAME_INVALID': 'Check that the result matches the configured number model and rules.', 'error.PLATFORM_GAME_CONFLICT': 'Already connected, duplicate record, or the draw is not ready.', 'error.PERMISSION_DENIED': 'Permission denied.', 'error.RESOURCE_NOT_FOUND': 'Game not found or not connected.', 'error.AUTH_SESSION_REVOKED': 'Please sign in again.', 'error.SERVICE_UNAVAILABLE': 'Service unavailable.', invalid: 'Check the entered values.',
 },
 'zh-CN': {
  backToGames: '返回彩种列表',
 editName: '修改彩种名称', ruleUnpublished: '未发布规则', rulePublished: '规则已发布', salesReady: '销售已启用', salesUnconfigured: '未配置销售', salesDisabled: '销售未启用', salesOutdated: '销售规则待更新', brandNotConnected: '本品牌未接入',

 drawConfiguration:'开奖配置', drawSources:'开奖数据源', manualResults:'手动开奖结果',
 gameActionPrompt: '确定要{action}“{name}”吗？',
 actions: '操作', status: '状态', active: '已上架', offline: '已下架', disabled: '本品牌已停用', enabled: '本品牌已启用', allStates: '全部状态', lifecycleFilter: '彩种状态', takeOffline: '下架', relist: '重新上架', deleteGame: '删除彩种', disableGame: '本品牌停用', enableGame: '本品牌启用', lifecycleConfirm: '确认彩种操作', confirmAction: '确认', cancelAction: '取消', offlineHint: '从用户列表隐藏，停止新投注及预约新自动期次。已有期次和注单继续开奖结算，历史记录和钱包流水保留。', brandDisableHint: '仅从本品牌用户列表隐藏并停止新投注。已有注单继续结算，历史记录和钱包流水保留。', enableHint: '恢复列表展示，仍按已有销售及自动开奖配置决定是否开放投注。', cannotDelete: '有未开奖期次或未结算订单，暂不能删除。', 'error.PLATFORM_GAME_IN_USE': '有未开奖期次或未结算订单，暂不能删除。',

 automation:'自动开奖',
 canada:'加拿大投注',
  markSix: '六合彩',
  playSelectionTitle: '选号与投注积分', playSelectionHelp: '设置玩家每个组合选择的普通号、特别号数量，彩种的号码模型保持不变。', playDigitsSelection: '按顺序选择 {count} 位数字，每位从 0～9 中选号。', playRegularCount: '每组合普通号数量', playSpecialCount: '每组合特别号数量', playUnit: '每组合投注积分',
  playTiersTitle: '中奖条件与派彩', playTiersHelp: "初始奖级为全命中，请核对条件和赔率。派彩＝每组合积分×赔率×投注倍数。", playTierNumber: '奖级 {count}', playRemoveTier: '删除奖级', playAddTier: '添加奖级', playOdds: '派彩倍数（赔率）', playTierCap: '本奖级派彩上限（积分）', playNoCap: '不设上限', playTierPayout: '奖级派彩方式', playExclusive: '排他：只取最高派彩', playAdditive: '累加：叠加本奖级派彩', playConditionLogic: '条件满足方式', playAllConditions: '全部条件满足', playAnyCondition: '任一条件满足', playConditionField: '判断内容', playDrawScope: '使用的开奖号码', playScopeAll: '全部开奖号码', playScopeDigits: '开奖数字', playComparison: '判断方式', playEquals: '等于', playBetween: '在范围内（含两端）', playValue: '数值', playMin: '最小值', playMax: '最大值', playRemoveCondition: '删除条件', playAddCondition: '添加条件', playMixedPolicy: '排他与累加奖级同时命中时', playChoosePolicy: '请选择派彩方式', playMixedMax: '只取所有奖级中的最高派彩', playMixedAdd: '最高排他派彩，加上全部累加派彩',
  playLimitsTitle: '限额与取整', playLimitsHelp: "组合上限限制复式展开数量；金额封顶可留空，最终积分四舍五入取整。", playOrderCap: '整单派彩上限（积分）', playMaxCombinations: '最大组合数量', playMaxMultiplier: '最大投注倍数', playMaxBet: '单笔投注上限（积分）', playRounding: '何时四舍五入', playRoundOrder: '整单汇总后', playRoundLine: '每个组合计算后', playRoundTier: '每个奖级计算后',
  playHelpTitle: '配置帮助', playHelpConditions: '命中数量用于比较玩家选号与开奖结果；其他判断可使用开奖总和、奇偶个数或号码跨度。“全部号码相同”填 1 表示是、0 表示否；跨度为最大号码减最小号码，最长连续号码段按个数计算。此表单配置的是号码选号玩法。', playHelpOdds: '例如：每组合 2 积分、派彩倍数 3、投注倍数 1，一个组合命中时派彩为 6 积分，再应用封顶和取整。初始赔率 1 仅为配置示例，请按业务确认后保存。', playHelpSave: "保存后不可修改；六合彩需另行发布销售配置才能开放投注。", playFormInvalid: '请检查选号数量、中奖条件、赔率、整数积分和限额；混用排他与累加奖级时，必须选择同时命中时的派彩方式。',
  'playField.position_match': '数字位置命中数', 'playField.regular_match': '普通号命中数', 'playField.special_match': '特别号命中数', 'playField.draw_sum': '开奖号码总和', 'playField.draw_odd_count': '奇数个数', 'playField.draw_even_count': '偶数个数', 'playField.draw_unique_count': '不同号码个数', 'playField.draw_all_same': '全部号码相同（1 是，0 否）', 'playField.draw_span': '最大号码减最小号码', 'playField.draw_consecutive': '最长连续号码段个数',
  calendarPreviousMonth: '上个月', calendarNextMonth: '下个月', calendarToday: '今天', timeHour: '时', timeMinute: '分', timeSecond: '秒', timeMeridiem: '上午/下午', timeAM: '上午', timePM: '下午',
  editModel: '修改号码模型', modelLocked: '已有玩法或期次的彩种，不能直接覆盖号码模型。', minimum: '最小号码', maximum: '最大号码', numberPool: '号码池', regularPool: '普通号码池', specialPool: '特别号码池', poolMode: '号码池定义', range: '号码范围', numberList: '号码列表（逗号分隔）',
  'error.MARK_SIX_ATTRIBUTES_MISSING': '请先发布开奖日期适用的年度号码属性，再创建期次。',
  'error.PLATFORM_GAME_MODEL_LOCKED': '彩种已有玩法或期次，号码模型不能直接覆盖。',
  'error.PLATFORM_GAME_VERSION_CONFLICT': '彩种已被更新，请重新打开号码模型后再修改。',
  ownership: '归属', platformOwner: '平台', loading: '加载中…',
  modelDigits: '有序数字（0–9）', modelXY: '普通号码＋特别号码', modelMN: '号码池选取',
  title: '平台彩种', name: '名称', code: '代码', timezone: '时区', periods: '期次', plays: '玩法规则', model: '号码模型', connect: '接入', connected: '已接入', create: '创建彩种', close: '关闭', refresh: '刷新', previous: '上一页', next: '下一页', empty: '暂无记录', reason: '原因', save: '保存', retry: '重试原请求', readonly: '平台统一维护号码模型、期次及开奖结果；接入品牌共用开奖，独立配置六合彩销售与限额。',
  type: '模型类型', digits: '位数', regularCount: '普通号码数量', specialCount: '特别号码数量', pool: '号码池最大值', repeat: '允许重复', ordered: '有序', result: '开奖结果', periodNo: '期号', start: '开售时间', end: '停售时间', drawAt: '开奖时间', date: '日期', time: '时间', pending: '尚未发布', publish: '发布开奖结果', createPeriod: '创建期次', createPlay: '新增玩法规则', definition: '规则定义', numbers: '号码（逗号分隔）', regular: '普通号码', special: '特别号码', awaiting: '请求结果尚未确认，请使用原请求重试。', success: '已保存',
  'error.NETWORK_ERROR': '连接失败，请重试原请求。', 'error.INVALID_RESPONSE': '服务器响应不正确。', 'error.PLATFORM_GAME_INVALID': '请检查开奖结果是否符合已配置的号码模型和规则。', 'error.PLATFORM_GAME_CONFLICT': '已接入、记录重复或尚未到开奖时间。', 'error.PERMISSION_DENIED': '无操作权限。', 'error.RESOURCE_NOT_FOUND': '彩种不存在或尚未接入。', 'error.AUTH_SESSION_REVOKED': '请重新登录。', 'error.SERVICE_UNAVAILABLE': '服务暂时不可用。', invalid: '请检查输入内容。',
 },
} as const
