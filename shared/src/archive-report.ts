export interface ArchiveReportRecord {
  id: string; revision: number; previous_id: string | null; snapshot_at: string; created_at: string;
  created_by: string | null; reason: string; payload_sha256: string; audit_log_id: string;
  automation?: { task_id: string; policy_version: number };
  window: { kind: 'daily' | 'monthly'; period_key: string; timezone: string; from: string; to: string };
  snapshot: { format_version: number; betting: object; ledger: object; withdrawals: object; commissions: object; rewards: object; reward_orders: object; wallet_snapshot: { at_snapshot: string; balances: object } };
}
export const archiveReportCatalog = {
  en: {
    title: 'Archive report', daily: 'Daily', monthly: 'Monthly', revision: 'Revision', back: 'Back to archives',
    overview: 'Business overview', commissionNet: 'Net commission points', walletTotal: 'Total wallet points', scopeNote: 'Financial totals use posting time. Order states are observed at the snapshot time for orders created during the reporting period.',
    from: 'From', to: 'To (exclusive)', timezone: 'Time zone', snapshotAt: 'Snapshot time',
    record: 'Archive record', reason: 'Reason', source: 'Source', manual: 'Manual', automatic: 'Automatic', createdAt: 'Created at',
    technical: 'Technical and audit information', id: 'Archive ID', previous: 'Previous archive ID', creator: 'Creator ID',
    hash: 'Payload SHA-256', audit: 'Audit ID', format: 'Format version', task: 'Task ID', policy: 'Policy version',
    walletNote: 'Wallet balances at the snapshot time, not flows during the reporting period.',
    withdrawalStatus: 'Status', count: 'Orders', points: 'Points', total: 'Total', reviewing: 'Reviewing', processing: 'Processing', paid: 'Paid', rejected: 'Rejected', failed: 'Failed', cancelled: 'Cancelled',
  },
  'zh-CN': {
    title: '归档报表', daily: '日报', monthly: '月报', revision: '版本', back: '返回归档列表',
    overview: '业务概览', commissionNet: '佣金净积分', walletTotal: '钱包总积分', scopeNote: '财务按实际入账时间统计；订单状态为统计期间创建的订单在快照时刻的状态。',
    from: '起始', to: '截止（不含）', timezone: '时区', snapshotAt: '快照时间',
    record: '归档记录', reason: '归档原因', source: '生成方式', manual: '人工生成', automatic: '自动生成', createdAt: '生成时间',
    technical: '技术与审计信息', id: '归档 ID', previous: '上一版本归档 ID', creator: '创建人 ID',
    hash: '载荷 SHA-256', audit: '审计 ID', format: '格式版本', task: '任务 ID', policy: '策略版本',
    walletNote: '钱包数据为快照时点的余额，不是统计期间的流水。',
    withdrawalStatus: '状态', count: '订单数', points: '积分', total: '合计', reviewing: '审核中', processing: '处理中', paid: '已付款', rejected: '已拒绝', failed: '失败', cancelled: '已取消',
  },
} as const
