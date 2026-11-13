import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { auditReasonLabel, hasAuditDetails, isAuditRead } from './audit-details'
import { businessActionLabel, businessResourceLabel } from './permission-labels'

describe('audit business presentation', () => {
  it('covers the literal audit identifiers emitted by the backend', () => {
    const root = resolve(import.meta.dirname, '../../backend/internal')
    for (const file of readdirSync(root, { recursive: true }) as string[]) {
      if (!file.endsWith('.go') || file.endsWith('_test.go')) continue
      const source = readFileSync(resolve(root, file), 'utf8')
      const actions = [...source.matchAll(/audit.Record\{[^}]*?Action:\s*"([a-z_]+(?:\.[a-z_]+)+)"(?!\s*\+)/g)].map(m => m[1])
      actions.push(...[...source.matchAll(/EntryType:\s*"([a-z_]+)"/g)].map(m => 'points.' + m[1]))
      const resources = [...source.matchAll(/ResourceType:\s*"([a-z_]+)"/g)].map(m => m[1])
      for (const locale of ['en', 'zh-CN'] as const) {
        for (const action of actions) expect(businessActionLabel(action, locale), `${file}: ${action}`).not.toMatch(/未识别|Unrecognized/)
        for (const resource of resources) expect(businessResourceLabel(resource, locale), `${file}: ${resource}`).not.toMatch(/未识别|Unrecognized/)
      }
    }
  })
  it('recognizes nested operation paths and query resources in both languages', () => {
    for (const locale of ['en', 'zh-CN'] as const) {
      for (const action of ['role.view', 'admin.view', 'association.select', 'filter.options.view', 'finance.recharge.confirm', 'reward.order.read', 'configured_sales.write', 'platform_game.period.opening', 'platform_game.automation.result', 'platform.role.create', 'report.commission.export', 'user.delete.platform']) {
        expect(businessActionLabel(action, locale)).not.toMatch(/未识别|Unrecognized|其他业务|Other business/)
        expect(businessResourceLabel('query', locale, action)).not.toMatch(/未识别|Unrecognized|其他业务|Other business/)
      }
    }
    expect(businessActionLabel('finance.recharge.confirm', 'zh-CN')).toBe('充值 · 编辑')
    expect(businessActionLabel('platform.role.create', 'en')).toBe('Platform roles · Create')
    expect(businessResourceLabel('query', 'zh-CN', 'role.view')).toBe('角色')
    expect(businessActionLabel('user.delete.maintenance', 'zh-CN')).toBe('会员 · 删除')
    expect(businessActionLabel('user.list', 'zh-CN')).toBe('会员 · 查询')
    expect(businessActionLabel('user.view', 'zh-CN')).toBe('会员 · 查看')
    expect(businessActionLabel('user.freeze', 'en')).toBe('Members · Edit')
    expect(businessActionLabel('unregistered.action', 'en')).toContain('Unrecognized')
  })
  it('only offers details that BusinessDetails can actually display', () => {
    for (const value of [null, {}, [], { request_id: 'internal' }, { password: 'secret' }, { config: {} }]) expect(hasAuditDetails(value)).toBe(false)
    for (const value of [{ enabled: false }, { points: 0 }, { name: '旧名称' }, JSON.stringify({ status: 'active' }), { config: { enabled: true } }]) expect(hasAuditDetails(value)).toBe(true)
    expect(isAuditRead('role.view', 'query')).toBe(true)
    expect(isAuditRead('reward.order.read', 'reward_order')).toBe(true)
    expect(isAuditRead('role.update', 'role')).toBe(false)
  })
})

it('localizes system settlement reasons without changing operator notes', () => {
  expect(auditReasonLabel('frozen configured rule settlement', 'zh-CN')).toBe('按投注时锁定的规则结算')
  expect(auditReasonLabel('frozen configured rule settlement', 'en')).toBe('Settlement using the rules locked at bet placement')
  for (const locale of ['en', 'zh-CN'] as const) {
    expect(auditReasonLabel('用户要求保留的完整原始附注', locale)).toBe('用户要求保留的完整原始附注')
    expect(auditReasonLabel('', locale)).toBe('—')
  }
})

it('localizes archive-generated reasons in both supported languages', () => {
  const cases = [
    ['Owned read-only platform browser verification', '平台只读浏览器验证', 'Platform read-only browser verification'],
    ['automatic report archive capture', '自动生成归档报表', 'Automatic report archive capture'],
    ['User requested removal of historical unnamed Telegram account', '用户请求删除历史未命名 Telegram 账号', 'User requested removal of historical unnamed Telegram account'],
    ['discover elapsed report period', '检测到已结束的报表周期', 'Detected an elapsed report period'],
  ] as const
  for (const [reason, zh, en] of cases) {
    expect(auditReasonLabel(reason, 'zh-CN')).toBe(zh)
    expect(auditReasonLabel(reason, 'en')).toBe(en)
  }
})

it('localizes configured ledger notes in both supported languages', () => {
  const cases = [
    ['confirmed configured stake', '确认投注扣减', 'Confirmed stake deduction'],
    ['configured loss from reserved original sources', '从原冻结来源扣除投注积分', 'Stake deducted from original reserved sources'],
    ['configured winnings including stake', '派发中奖积分（含本金）', 'Winnings credited including stake'],
    ['configured push original sources', '和局退回原积分来源', 'Push refunded to original sources'],
  ] as const
  for (const [reason, zh, en] of cases) {
    expect(auditReasonLabel(reason, 'zh-CN')).toBe(zh)
    expect(auditReasonLabel(reason, 'en')).toBe(en)
  }
})

it('covers generated audit and ledger notes emitted by current services and initialization', () => {
  const root = resolve(import.meta.dirname, '../../backend/internal')
  const notes = new Set<string>(['COMPLIANCE_REVIEW_REQUIRED'])
  const transitions = /\b(?:appendCycleStep|transitionPayment|correctionPlanStep|correctionExecutionStep|discoveryTransition|finishAutomatic|publishAutomaticResult|orderEvidence|advanceLocked)\(/
  for (const file of readdirSync(root, { recursive: true }) as string[]) {
    if (!file.endsWith('.go') || file.endsWith('_test.go')) continue
    const source = readFileSync(resolve(root, file), 'utf8')
    for (const line of source.split('\n')) {
      for (const match of line.matchAll(/Reason:\s*"([^"\n]* [^"\n]+)"/g)) notes.add(match[1])
      if (transitions.test(line) || /\b(?:const reason|reason\s*:=|state, operation, reason)/.test(line)) {
        for (const match of line.matchAll(/"([^"\n]*)"/g)) if (match[1].includes(" ")) notes.add(match[1])
      }
    }
  }
  const baseline = readFileSync(resolve(root, '../migrations/0001_baseline.up.sql'), 'utf8')
  for (const match of baseline.matchAll(/'(Initial [^']+|Automatic member referral allocation)'/g)) notes.add(match[1])
  expect(notes.size).toBeGreaterThan(40)
  for (const note of notes) {
    expect(auditReasonLabel(note, 'zh-CN'), note).toMatch(/[\u4e00-\u9fff]/)
    expect(auditReasonLabel(note, 'en'), note).toMatch(/[A-Za-z]/)
  }
})

it('preserves free-form notes in either language', () => {
  for (const note of ['Keep this English operator note unchanged.', '人工审核备注：允许提现。']) {
    expect(auditReasonLabel(note, 'en')).toBe(note)
    expect(auditReasonLabel(note, 'zh-CN')).toBe(note)
  }
  expect(auditReasonLabel('withdrawal application', 'zh-CN')).toBe('提交提现申请')
  expect(auditReasonLabel('automatic review by saved policy', 'en')).toBe('Automatically reviewed using the saved settings')
})
