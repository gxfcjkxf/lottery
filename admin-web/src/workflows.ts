export type RuleStatus = 'draft' | 'pending_review' | 'approved' | 'rejected'

export interface RuleDraft {
  creator: string
  reviewer: string
  status: RuleStatus
  testSelection: string
  testResult: string
  unitPoints: string
  multiplier: string
}

export function simulateRule(draft: RuleDraft) {
  const picks = draft.testSelection.split(/[ ,，]+/).filter(Boolean)
  const result = draft.testResult.split(/[ ,，]+/).filter(Boolean)
  const unique = new Set(picks)
  const selectionValid = picks.length > 0 && unique.size === picks.length && picks.every((pick) => /^\d{1,2}$/.test(pick))
  const unitValid = /^(?:0|[1-9]\d*)$/.test(draft.unitPoints)
  const multiplierValid = /^(?:[1-9]\d?|100)$/.test(draft.multiplier)
  const valid = selectionValid && unitValid && multiplierValid
  const matches = valid ? picks.filter((pick) => result.includes(pick)).length : 0
  const points = valid ? (BigInt(draft.unitPoints) * BigInt(draft.multiplier) * BigInt(matches)).toString() : '0'
  const explanation = !selectionValid ? '选号需为不重复的数字，至少填写一个' : !unitValid ? '单注积分必须是非负整数的十进制字符串' : !multiplierValid ? '倍数必须为 1 到 100 的整数' : `${matches} 个选号命中测试结果`
  return { valid, matches, points, combinations: String(valid ? unique.size : 0), explanation }
}

export function submitRuleForReview(draft: RuleDraft): RuleDraft {
  if (draft.status !== 'draft' && draft.status !== 'rejected') throw new Error('只有草稿或已驳回规则可以提交审核')
  if (draft.creator === draft.reviewer) throw new Error('创建者不能审核自己的规则；请提交给其他审核人')
  return { ...draft, status: 'pending_review' }
}

export function reviewRule(draft: RuleDraft, reviewer: string, decision: 'approve' | 'reject'): RuleDraft {
  if (draft.status !== 'pending_review') throw new Error('当前规则不在待审核状态')
  if (reviewer === draft.creator) throw new Error('创建者不能审核自己的规则')
  return { ...draft, reviewer, status: decision === 'approve' ? 'approved' : 'rejected' }
}

export function previewCorrection(oldResult: string, newResult: string, affectedOrders: number) {
  const reasonRequired = oldResult.trim() !== newResult.trim()
  return { oldResult, newResult, affectedOrders: String(affectedOrders), estimatedSettlements: String(affectedOrders), reasonRequired }
}
