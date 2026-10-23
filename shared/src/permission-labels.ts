const modules: Record<string, [string, string]> = {
	overview: ['经营总览', 'Platform overview'],
  platform_game: ['平台彩种', 'Platform games'],
  admin: ['账号', 'Accounts'], agent: ['代理', 'Agents'], agent_policy: ['代理政策', 'Agent policy'], audit: ['审计', 'Audit'], auth_config: ['认证配置', 'Authentication settings'],
  bet: ['注单', 'Bet orders'], bet_policy: ['投注政策', 'Betting policy'], brand: ['品牌', 'Brands'], brand_domains: ['品牌域名', 'Brand domains'], brand_operation: ['品牌运营', 'Brand operations'], brand_presentation: ['品牌展示', 'Brand presentation'],
  commission: ['佣金', 'Commission'], commission_adjustment: ['佣金调整', 'Commission adjustments'], commission_correction: ['佣金更正', 'Commission corrections'], commission_correction_policy: ['佣金更正政策', 'Commission correction policy'], commission_payment: ['佣金支付', 'Commission payments'], commission_payment_policy: ['佣金支付政策', 'Commission payment policy'], commission_policy: ['佣金政策', 'Commission policy'], platform_settings: ['平台设置', 'Platform settings'],
  compliance_check: ['合规检查', 'Compliance checks'], compliance_policy: ['合规政策', 'Compliance policy'], draw: ['开奖', 'Draws'], draw_source: ['开奖来源', 'Draw sources'], game: ['彩种', 'Games'], join_code: ['推荐码与代理码', 'Referral & agent codes'], notification: ['通知', 'Notifications'], notification_template: ['通知模板', 'Notification templates'], period: ['期次', 'Periods'], point_policy: ['积分政策', 'Point policy'], recharge: ['充值', 'Recharges'],
  report_archive: ['报表归档', 'Report archives'], report_archive_policy: ['报表归档政策', 'Report archive policy'], report_archive_task: ['报表归档任务', 'Report archive tasks'], report_attribution: ['归因报表', 'Attribution reports'], report_betting: ['投注报表', 'Betting reports'], report_commission: ['佣金报表', 'Commission reports'], report_ledger: ['账本报表', 'Ledger reports'], report_reward: ['奖励报表', 'Reward reports'], report_withdrawal: ['提现报表', 'Withdrawal reports'], reward: ['奖励', 'Rewards'], role: ['角色', 'Roles'], rule: ['规则', 'Rules'], schedule: ['日程', 'Schedules'], settlement: ['结算', 'Settlement'], settlement_policy: ['结算政策', 'Settlement policy'], user: ['会员', 'Members'], wallet: ['钱包', 'Wallet'], withdrawal: ['提现', 'Withdrawals'], withdrawal_policy: ['提现政策', 'Withdrawal policy'],
}

const actions: Record<string, [string, string]> = {
  kick: ['强制下线', 'Force sign out'],
  connect: ['接入', 'Connect'], sell: ['销售配置', 'Sales configuration'],
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


/** Audit history uses the same business vocabulary as access management. */
export function businessResourceLabel(resource: string, locale: 'en' | 'zh-CN'): string {
  const labels = modules[resource]
  return labels ? labels[locale === 'zh-CN' ? 0 : 1] : (locale === 'zh-CN' ? '其他业务' : 'Other business activity')
}
export function businessActionLabel(action: string, locale: 'en' | 'zh-CN'): string {
  const [resource, operation] = action.split('.')
  const labels = actions[operation === 'update' ? 'write' : operation]
  const name = businessResourceLabel(resource, locale)
  return labels ? `${name} · ${labels[locale === 'zh-CN' ? 0 : 1]}` : name
}
