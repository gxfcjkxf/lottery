import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'

const read = path => readFileSync(new URL(`../../${path}`, import.meta.url), 'utf8')

test('current handover uses the single baseline and independent platform creation route', () => {
  const readme = read('README.md')
  assert.match(readme, /0001_baseline\.up\.sql/)
  assert.match(readme, /POST \/api\/v1\/platform\/brands/)
  assert.doesNotMatch(readme, /POST \/api\/v1\/admin\/brands|migrate至00\d\d|0018–0020|原型演示订单|用户投注\/提现与业务订单仍待/)
  assert.match(readme, /platform-web\s+总后台/)
  assert.match(readme, /APP_ENV=development/)
  assert.match(readme, /总管理员`admin \/ admin123`/)
  assert.match(readme, /test\/production不自动创建管理员/)
  assert.doesNotMatch(readme, /没有admin\/admin123或其他内置密码|在production拒绝运行，不创建默认密码/)
})

test('current module handovers do not instruct installation of removed incremental migrations', () => {
  const historical = new Set(['implementation-progress.md', 'requirements-draft.md'])
  for (const name of readdirSync(new URL('../../docs/', import.meta.url)).filter(name => name.endsWith('.md') && !historical.has(name))) {
    assert.doesNotMatch(read(`docs/${name}`), /migrate\s*(?:至|到)\s*00\d\d|升级至\s*00\d\d|正常迁移\s*00\d\d/, name)
  }
  for (const name of ['19-commission-posting-reports', '21-reward-reports', '23-commission-correction-execution', '25-commission-correction-observability', '26-report-archive-core', '27-automatic-report-archive-core', '28-report-archive-task-management', '29-report-archive-activation', '37-brand-business-inventory', '38-commission-cycle-analysis']) {
    assert.match(read(`docs/${name}.md`), /0001_baseline\.up\.sql/)
    assert.match(read(`docs/${name}.md`), /README\.md#空库安装/)
  }
  assert.match(read('docs/README.md'), /备份恢复和审计仍保留/)
})

test('default infrastructure starts only the implemented PostgreSQL dependency', () => {
  assert.match(read('Makefile'), /infra:\n\tdocker compose up -d postgres\n/)
  assert.match(read('README.md'), /docker compose up -d postgres/)
  assert.doesNotMatch(read('.env.example'), /^(?:REDIS_URL|NATS_URL|OBJECT_STORAGE_\w+)=/m)
})

test('commission handovers use current formats rather than removed recovery flows', () => {
  const workbench = read('docs/36-commission-workbench.md')
  assert.match(workbench, /旧not_implemented\/null快照不受支持/)
  assert.doesNotMatch(workbench, /保留可读兼容|历史blocked计划可能保留原未决政策错误码/)
  assert.match(read('admin-web/src/workbench-api.ts'), /const STATUSES = \["ready", "forbidden"\]/)

  const analysis = read('docs/38-commission-cycle-analysis.md')
  const csvSource = read('backend/internal/reporting/commission_analysis_csv.go')
  const fields = csvSource.match(/var commissionAnalysisCSVFields = \[\]string\{([\s\S]*?)\n\}/)?.[1].match(/"[a-z_]+"/g)
  assert.equal(fields?.length, 33)
  assert.match(analysis, /三个coverage字段/)
  assert.match(analysis, /共33列/)
  assert.doesNotMatch(analysis, /34列|COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED|升级必须停止/)

  const acceptance = read('docs/08-acceptance-and-open-items.md')
  assert.doesNotMatch(acceptance, /34列|MODE_UNRESOLVED|COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED|retryPlan流程/)
  assert.match(acceptance, /`platform-web` 独立总后台/)
  const index = read('docs/README.md')
  assert.doesNotMatch(index, /旧空快照兼容|历史未决计划显式审计重试|旧错误stale升级拒绝/)
  const backlog = read('docs/09-implementation-backlog.md')
  assert.doesNotMatch(backlog, /历史未决计划|历史计划恢复|复用0006结构|0071升级/)
  assert.match(backlog, /真实技术失败的显式审计重试/)
  assert.match(backlog, /当前单份完整基线/)
})

test('brand administration metadata and authenticated header do not label real operations as a prototype', () => {
  const html = read('admin-web/index.html')
  assert.match(html, /<title>Brand administration<\/title>/)
  assert.doesNotMatch(html, /演示|原型|所有数据仅/)
  const app = read('admin-web/src/App.vue')
  assert.match(app, /ui\("品牌后台"\)/)
  assert.doesNotMatch(app, /ui\("演示原型"\)/)
})

test('current UI handover describes implemented modules and confirmed withdrawal rules', () => {
  const spec = read('docs/05-ui-spec.md')
  assert.match(spec, /管理平台管理员与品牌员工账号及品牌角色/)
  assert.match(spec, /佣金与奖励报表、不可变日\/月归档已接入/)
  assert.match(spec, /自动归档配置、任务查询和失败人工重试已接入/)
  assert.match(spec, /接入二十二类事件/)
  assert.match(spec, /N 必须大于0，null 表示继承，0不是有效配置/)
  assert.match(spec, /充值、中奖、佣金、赠送来源/)
  assert.match(spec, /具备所属品牌审核权限的创建者或编辑者允许自审/)
  assert.doesNotMatch(spec, /创建者及所有编辑者不能自审|期次、用户总额度和取消业务仍待 S5 接入/)
  assert.match(read('docs/06-architecture.md'), /平台管理员、品牌员工账号和品牌角色管理/)
  assert.match(read('docs/01-product-spec.md'), /四来源投注扣款顺序固定为/)
  assert.doesNotMatch(read('docs/01-product-spec.md'), /彩种配置可覆盖并须审核/)
  assert.match(read('docs/09-implementation-backlog.md'), /Redis、外部消息队列、对象存储为后续扩展/)
  assert.doesNotMatch(spec, /佣金\/奖励及不可变日月结仍未接入|自动任务仍未接入|接入八类事件|金额分配明确使用三种来源|旧not_implemented\/null快照/)
  assert.match(read('admin-web/src/ReportsManagement.vue'), /<CommissionReport[\s\S]*<RewardReports/)
  assert.match(read('admin-web/src/App.vue'), /<ReportArchivesManagement[\s\S]*<ReportArchiveTasksManagement/)
  assert.match(read('user-web/src/WithdrawalPanel.vue'), /const sources: WithdrawalSource\[\] = \["recharge", "winning", "gift", "commission"\]/)
})
