const modules: Record<string, [string, string]> = {
 data_retention: ['数据归档','Data archiving'],data_retention_policy:['归档设置','Archive settings'],data_retention_task:['归档任务','Retention execution'],
	announcement: ['公告', 'Announcements'],
	overview: ['经营总览', 'Platform overview'],
  platform_game: ['平台彩种', 'Platform games'],
  admin: ['账号', 'Accounts'], agent: ['代理', 'Agents'], agent_policy: ['代理设置', 'Agent settings'], audit: ['审计', 'Audit'], auth_config: ['认证配置', 'Authentication settings'],
  bet: ['注单', 'Bet orders'], bet_policy: ['投注设置', 'Betting settings'], brand: ['品牌', 'Brands'], brand_domains: ['品牌域名', 'Brand domains'], brand_operation: ['品牌运营', 'Brand operations'], brand_presentation: ['品牌展示', 'Brand presentation'],
  commission: ['佣金', 'Commission'], commission_adjustment: ['佣金调整', 'Commission adjustments'], commission_correction: ['佣金更正', 'Commission corrections'], commission_correction_policy: ['佣金更正设置', 'Commission correction settings'], commission_payment: ['佣金支付', 'Commission payments'], commission_payment_policy: ['佣金支付设置', 'Commission payment settings'], commission_policy: ['佣金设置', 'Commission settings'], platform_settings: ['平台设置', 'Platform settings'],
  draw: ['开奖', 'Draws'], draw_source: ['开奖来源', 'Draw sources'], game: ['彩种', 'Games'], join_code: ['推荐码与代理码', 'Referral & agent codes'], notification: ['通知', 'Notifications'], notification_template: ['通知模板', 'Notification templates'], period: ['期次', 'Periods'], point_policy: ['积分设置', 'Point settings'], recharge: ['充值', 'Recharges'], recharge_quota: ['品牌充值额度', 'Brand recharge quota'],
  report_archive: ['报表归档', 'Report archives'], report_archive_policy: ['报表归档设置', 'Report archive settings'], report_archive_task: ['报表归档任务', 'Report archive tasks'], report_attribution: ['归因报表', 'Attribution reports'], report_betting: ['投注报表', 'Betting reports'], report_commission: ['佣金报表', 'Commission reports'], report_ledger: ['账本报表', 'Ledger reports'], report_reward: ['奖励报表', 'Reward reports'], report_withdrawal: ['提现报表', 'Withdrawal reports'], reward: ['奖励', 'Rewards'], role: ['角色', 'Roles'], rule: ['规则', 'Rules'], schedule: ['日程', 'Schedules'], settlement: ['结算', 'Settlement'], settlement_policy: ['结算设置', 'Settlement settings'], user: ['会员', 'Members'], wallet: ['钱包', 'Wallet'], withdrawal: ['提现', 'Withdrawals'], withdrawal_policy: ['提现设置', 'Withdrawal settings'],
}

const actions: Record<string, [string, string]> = {
  publish: ['发布与下架', 'Publish and take down'],
  kick: ['强制下线', 'Force sign out'],
  connect: ['接入', 'Connect'], sell: ['投注配置', 'Betting configuration'],
  view: ['查看', 'View'], write: ['编辑', 'Edit'], create: ['创建', 'Create'], delete: ['删除', 'Delete'], export: ['导出', 'Export'], download: ['下载', 'Download'],
  grant: ['发放', 'Grant'], revoke: ['撤销', 'Revoke'], retry: ['重试', 'Retry'], run: ['执行', 'Run'], approve: ['审批', 'Approve'], preview: ['预览', 'Preview'],
  cancel: ['取消', 'Cancel'], cancel_retry: ['取消重试', 'Cancel retry'], judge_cancel: ['判定取消', 'Judge cancellation'], mark_abnormal: ['标记异常', 'Mark abnormal'], correct: ['更正', 'Correct'], correction_retry: ['更正重试', 'Correction retry'],
  manual_create: ['手动创建', 'Create manually'], generate: ['生成', 'Generate'], fail: ['标记失败', 'Mark failed'], mark_paid: ['标记已支付', 'Mark paid'], reject: ['驳回', 'Reject'],
  password_reset: ['重置密码', 'Reset password'], freeze: ['冻结', 'Freeze'], adjust: ['调整', 'Adjust'], repair: ['修复', 'Repair'], reconcile: ['对账', 'Reconcile'], simulate: ['模拟', 'Simulate'],
  validate: ['校验', 'Validate'], submit: ['提交', 'Submit'], review: ['审核', 'Review'], continue: ['继续', 'Continue'], execute_retry: ['执行重试', 'Execute retry'],
}

export function permissionLabel(key: string, locale: 'en' | 'zh-CN'): string {
  return `${permissionGroupLabel(key, locale)} · ${permissionActionLabel(key, locale)}`
}

export function permissionGroupLabel(key: string, locale: 'en' | 'zh-CN'): string {
  const moduleLabel = modules[key.split('.')[0]]
  if (!moduleLabel) throw new Error(`Missing permission translation: ${key}`)
  return moduleLabel[locale === 'zh-CN' ? 0 : 1]
}

export function permissionActionLabel(key: string, locale: 'en' | 'zh-CN'): string {
  const [module, action] = key.split('.')
  const moduleLabel = modules[module]
  const actionLabel = actions[action]
  if (!moduleLabel || !actionLabel) throw new Error(`Missing permission translation: ${key}`)
  return actionLabel[locale === 'zh-CN' ? 0 : 1]
}


/** Audit identifiers are structured paths; resolve the longest business prefix. */
const auditResources: Record<string, [string, string]> = {
 cash:["资金钱包","Currency wallet"],
 "cash.config":["币种设置","Currency settings"],
 "cash.destination":["收款账号","Payment accounts"],
 "cash.deposit":["充值","Deposits"],
 "cash.withdrawal":["提现","Withdrawals"],
 "cash.order":["资金订单","Currency orders"],
 "cash.balances":["资金余额","Currency balances"],
  ...modules,
  'platform.data_retention.policy':['归档设置','Archive settings'],
  'platform.data_retention.task':['归档任务','Retention execution'],
  association: ['关联账号搜索', 'Account search'], filter: ['筛选选项', 'Filter options'],
  brand_contacts: ['品牌联系方式', 'Brand contact details'],
  'brand_contacts.image': ['联系图片', 'Contact image'],
  'brand_presentation.logo': ['品牌标志素材', 'Brand logo asset'],

  'query': ['业务查询', 'Business query'],
  'permission': ['访问权限', 'Access permissions'],
  'access': ['访问权限', 'Access permissions'],
  'auth': ['身份验证', 'Authentication'],
  'member': ['会员', 'Members'],
  'brand_member': ['品牌会员', 'Brand members'],
  'global_user': ['全局会员', 'Global members'],
  'agent_account': ['代理账号', 'Agent accounts'],
  'agent_node': ['代理', 'Agents'],
  'audit_export': ['审计导出', 'Audit export'],
  'brand_point_policy': ['积分设置', 'Point settings'],
  'brand_withdrawal_policy': ['品牌提现设置', 'Brand withdrawal settings'],
  'game_withdrawal_policy': ['彩种提现设置', 'Game withdrawal settings'],
  'point_account': ['积分账户', 'Points account'],
  'points': ['积分', 'Points'],
  'recharge_order': ['充值单', 'Recharge order'],
  'recharge_quota_entry': ['充值额度流水', 'Recharge quota entry'],
  'finance.recharge': ['充值', 'Recharges'],
  'finance.recharge_quota': ['品牌充值额度', 'Brand recharge quota'],
  'configured': ['注单', 'Bet orders'],
  'configured_order': ['注单', 'Bet order'],
  'configured_rule': ['彩种规则', 'Game rules'],
  'configured_sales': ['彩种投注配置', 'Game betting settings'],
  'bet_limits': ['投注限额', 'Bet limits'],
  'withdrawal_order': ['提现单', 'Withdrawal order'],
  'reward_order': ['奖励单', 'Reward order'],
  'reward.order': ['奖励单', 'Reward order'],
  'notification_delivery': ['通知投递', 'Notification delivery'],
  'notification.template': ['通知模板', 'Notification templates'],
  'report_export': ['报表导出', 'Report export'],
  'hkjc_calendar': ['香港赛马会日历', 'HKJC calendar'],
  'platform.role': ['平台角色', 'Platform roles'],
  'platform_game_period': ['彩种期次', 'Game period'],
  'brand.platform_game': ['品牌彩种', 'Brand games'],
  'platform_game.name': ['彩种名称', 'Game name'],
  'platform_game.model': ['号码模型', 'Number model'],
  'platform_game.play': ['彩种玩法', 'Game plays'],
  'platform_game.period': ['彩种期次', 'Game periods'],
  'platform_game.result': ['开奖结果', 'Draw results'],
  'platform_game.draw_source': ['开奖来源', 'Draw sources'],
  'platform_game.mark_six': ['六合彩属性', 'Mark Six attributes'],
  'platform_game.automation': ['自动开奖设置', 'Automatic draw settings'],
  'platform_game.schedule': ['开奖日程', 'Draw schedule'],
  'platform_game.calendar': ['开奖日历', 'Draw calendar'],
  'platform_auth_settings': ['平台认证默认', 'Platform authentication defaults'],
  'auth_config.platform': ['平台认证默认', 'Platform authentication defaults'],
  'platform_commission_policy': ['平台佣金设置', 'Platform commission settings'],
  'platform_report_archive_policy': ['平台归档设置', 'Platform archive settings'],
  'workbench': ['工作台', 'Workbench'],
  'report': ['报表', 'Reports'],
  'commission_cycle': ['佣金周期', 'Commission cycle'],
  'commission_discovery': ['佣金发现任务', 'Commission discovery'],
  'commission_correction_execution': ['佣金更正执行', 'Commission correction execution'],
  'commission_correction_plan': ['佣金更正计划', 'Commission correction plan'],
  'commission_correction_target': ['佣金更正对象', 'Commission correction target'],
  'commission_payment_target': ['佣金支付对象', 'Commission payment target'],
}
const auditOperations: Record<string, [string, string]> = {
 agent_transfer_credit:["代理转入","Agent transfer received"], agent_transfer_debit:["转给会员","Transfer to member"], transfer:["转移","Transfer"],
 currency_exchange_credit:["资金转积分","Convert funds to points"], currency_exchange_debit:["积分转资金","Convert points to currency"], exchange:["兑换","Exchange"], config:["配置","Configure"],
  ...actions,
  withdrawal_reserve: ['占用提现积分', 'Reserve withdrawal points'], withdrawal_paid: ['扣除已付提现积分', 'Debit paid withdrawal points'], withdrawal_release: ['退还提现占用积分', 'Release withdrawal points'],
  reward_grant: ['奖励入账', 'Credit reward'], reward_reversal: ['撤销奖励积分', 'Reverse reward points'],

  manual: ['补录开奖结果', 'Add historical draw result'],
  result: ['发布开奖结果', 'Publish draw result'],
  post: ['入账', 'Post'], invalidate: ['标记失效', 'Invalidate'], register: ['登记', 'Register'], failure: ['执行失败', 'Execution failed'],
  page: ['处理批次', 'Process batch'], apply: ['执行调整', 'Apply adjustment'], start_run: ['开始核算批次', 'Start calculation run'],
  manifest_page: ['保存注单批次', 'Save order batch'], calculation_page: ['核算批次', 'Calculate batch'], earnings_page: ['汇总收益批次', 'Sum earnings batch'],
  reclaim: ['收回额度', 'Reclaim quota'], manual_recharge: ['人工充值扣额', 'Debit manual recharge quota'], manual_adjustment: ['人工调整额度', 'Adjust quota manually'],
  processing: ['处理中', 'Processing'], failed: ['执行失败', 'Failed'],

  check: ['检查', 'Check'], recharge: ['充值入账', 'Credit recharge'], bet: ['投注扣减', 'Deduct stake'], prize: ['中奖入账', 'Credit prize'], commission: ['佣金入账', 'Credit commission'],
  commission_adjustment: ['调整佣金', 'Adjust commission'], commission_correction: ['更正佣金', 'Correct commission'], reversal: ['冲正', 'Reverse'],

  me: ['查看本人资料', 'View own profile'], select: ['搜索选择', 'Search and select'], defaults: ['查看默认配置', 'View defaults'], upload: ['上传', 'Upload'],
  actions: ['查看操作记录', 'View actions'], targets: ['查看明细', 'View targets'], earnings: ['查看收益', 'View earnings'], runs: ['查看执行批次', 'View runs'], calculations: ['查看核算明细', 'View calculations'],
  run_allocations: ['查看分配明细', 'View allocations'], run_calculations: ['查看批次核算', 'View run calculations'],
  retry_plan: ['重新生成计划', 'Regenerate plan'], start: ['开始', 'Start'], complete: ['完成', 'Complete'], pause: ['暂停', 'Pause'], resume: ['继续', 'Resume'],
  calculate: ['核算', 'Calculate'], enumerate: ['枚举', 'Enumerate'], ready: ['准备完成', 'Ready'], finish: ['完成', 'Finish'],
  grant_retry: ['重试发放', 'Retry grant'], cancel_pending: ['待取消', 'Cancellation pending'], payment_pending: ['待支付', 'Payment pending'],
  paid_external: ['外部支付完成', 'External payment completed'], submitted: ['已提交', 'Submitted'], reviewing: ['审核中', 'Under review'],

  update: ['编辑', 'Edit'], read: ['查看', 'View'], list: ['列表查询', 'List'], history: ['查看历史', 'View history'],
  save: ['保存', 'Save'], publish: ['发布', 'Publish'], confirm: ['确认', 'Confirm'],
  login: ['登录', 'Sign in'], logout: ['退出登录', 'Sign out'], password_change: ['修改密码', 'Change password'],
  operator_create: ['后台创建', 'Create by operator'], profile_fill: ['补充资料', 'Complete profile'], telegram_bind: ['绑定 Telegram', 'Bind Telegram'],
  rate_limit: ['限制访问频率', 'Rate limit'], denied: ['访问被拒绝', 'Access denied'],
  place: ['投注', 'Place bet'], settle: ['结算', 'Settle'], settled: ['已结算', 'Settled'],
  lifecycle: ['修改上架状态', 'Change listing status'], availability: ['修改可用状态', 'Change availability'],
  opening: ['设置开盘时间', 'Set opening time'], attributes: ['设置属性', 'Set attributes'], sync: ['同步', 'Sync'],
  sales: ['设置投注', 'Configure betting'], enqueue: ['加入任务队列', 'Enqueue task'], automatic: ['自动创建', 'Create automatically'],
  target: ['处理明细', 'Process target'], pending: ['待处理', 'Pending'], paid: ['已支付', 'Paid'], rejected: ['已驳回', 'Rejected'],
  approved: ['已审批', 'Approved'], cancelled: ['已取消', 'Cancelled'], paused: ['已暂停', 'Paused'],
  credit: ['增加额度', 'Credit'], debit: ['扣减额度', 'Debit'], reserve: ['占用', 'Reserve'], release: ['释放', 'Release'],
  consume: ['扣除', 'Consume'], refund: ['退还', 'Refund'], create_user: ['创建会员', 'Create member'],
}
function auditResourceKey(path: string): string | undefined {
  const normalized = path.replaceAll('.', '_')
  if (auditResources[path]) return path
  if (auditResources[normalized]) return normalized
  const parts = path.split('.')
  while (parts.length > 1) {
    parts.pop()
    const prefix = parts.join('.')
    if (auditResources[prefix]) return prefix
    if (auditResources[parts.join('_')]) return parts.join('_')
  }
  return undefined
}
export function businessResourceLabel(resource: string, locale: 'en' | 'zh-CN', action?: string): string {
  const key = auditResourceKey(resource === 'query' && action ? action : resource)
  return key ? auditResources[key][locale === 'zh-CN' ? 0 : 1] : (locale === 'zh-CN' ? '未识别的资源' : 'Unrecognized resource')
}
export function businessActionLabel(action: string, locale: 'en' | 'zh-CN'): string {
  const parts = action.split('.')
  if (parts.at(-1) === 'platform' || parts.at(-1) === 'brand' || parts.at(-1) === 'maintenance') parts.pop()
  const operation = parts.pop() ?? ''
  const labels = auditOperations[operation]
  const name = businessResourceLabel(parts.join('.'), locale)
  // Unknown identifiers remain explicitly unrecognized instead of disguising them as another operation.
  const category = ['list', 'select', 'enumerate'].includes(operation) ? ['查询', 'Query']
    : ['view', 'read', 'history', 'me', 'defaults', 'actions', 'targets', 'earnings', 'runs', 'calculations', 'run_allocations', 'run_calculations', 'preview', 'download', 'export', 'check', 'validate', 'simulate'].includes(operation) ? ['查看', 'View']
    : ['create', 'operator_create', 'manual_create', 'create_user', 'automatic', 'generate', 'place', 'submit', 'register'].includes(operation) ? ['创建', 'Create']
    : operation === 'delete' ? ['删除', 'Delete'] : ['编辑', 'Edit']
  const verb = labels ? category[locale === 'zh-CN' ? 0 : 1] : (locale === 'zh-CN' ? '未识别的操作' : 'Unrecognized operation')
  return `${name} · ${verb}`
}
