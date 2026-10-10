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
  assert.doesNotMatch(spec, /佣金\/奖励及不可变日月结仍未接入|自动任务仍未接入|接入八类事件|金额分配明确使用三种来源|旧not_implemented\/null快照/)
  assert.match(read('admin-web/src/ReportsManagement.vue'), /<CommissionReport[\s\S]*<RewardReports/)
  assert.match(read('admin-web/src/App.vue'), /<ReportArchivesManagement[\s\S]*<ReportArchiveTasksManagement/)
  assert.match(read('user-web/src/WithdrawalPanel.vue'), /const sources: WithdrawalSource\[\] = \["recharge", "winning", "gift", "commission"\]/)
})
