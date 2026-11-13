const catalog = {
 en: {
  daily: 'Daily archive', monthly: 'Monthly archive', enabled: 'Enabled', executionTime: 'Execution time', executionDay: 'Day of month',
  dailyOffset: 'Archive days earlier', monthlyOffset: 'Archive months earlier',
  nextRun: 'Next run', target: 'Target period',
  nextStart: 'Changes apply from the next scheduled execution. Completed periods are captured at execution time.',
  invalid: 'Enter a valid time, day (1–31), daily offset (1–365), and monthly offset (1–120).',
  retry: 'Retry original request', unknown: 'The result is unknown. Retry the original request to confirm.', discardConflict: 'Reload and discard rejected request',
  saved: 'Archive configuration saved.', readOnly: 'You have read-only access.', review: 'Confirm archive configuration', confirm: 'I have checked the execution times and target periods.',
  disabled: 'Disabled', save: 'Save configuration', cancel: 'Cancel', reload: 'Reload configuration',
 },
 'zh-CN': {
  daily: '日归档', monthly: '月归档', enabled: '启用', executionTime: '执行时间', executionDay: '每月执行日',
  dailyOffset: '归档前几天', monthlyOffset: '归档前几个月',
  nextRun: '下次执行', target: '归档周期',
  nextStart: '配置从下次计划执行开始生效，在执行时保存目标周期的数据快照。',
  invalid: '请填写有效时间、执行日（1–31）、日周期（1–365）和月周期（1–120）。',
  retry: '重试原请求', unknown: '操作结果未知，请重试原请求确认。', discardConflict: '重新读取并丢弃冲突请求',
  saved: '归档配置已保存。', readOnly: '当前账号仅可查看配置。', review: '确认归档配置', confirm: '我已核对执行时间与归档周期。',
  disabled: '未启用', save: '保存配置', cancel: '取消', reload: '重新读取配置',
 },
} as const;
export function archiveScheduleCopy(locale: 'en' | 'zh-CN') { return catalog[locale]; }
