import { businessDetailsCatalog } from './business-details-catalog'

export function auditSnapshot(value: unknown): unknown {
  if (typeof value === 'string') return JSON.parse(value)
  return value
}
const sensitive = /password|secret|token|authorization|cookie|credential|private|api[_-]?key|salt|email|phone|mobile|address|birth|identity|ssn/i
/** Match the business fields actually rendered by BusinessDetails, excluding credentials. */
export function hasAuditDetails(value: unknown): boolean {
  return visible(auditSnapshot(value))
}
function visible(parsed: unknown): boolean {
  if (Array.isArray(parsed)) return parsed.some(visible)
  if (parsed && typeof parsed === 'object') return Object.entries(parsed).some(([key, child]) =>
    !sensitive.test(key) && Object.hasOwn(businessDetailsCatalog.en, 'field.' + key) && visible(child))
  return parsed !== null && parsed !== undefined && parsed !== ''
}
export function isAuditRead(action: string, resource: string): boolean {
  return resource === 'query' || /\.(view|read|list|history)(\.|$)/.test(action) || action.endsWith('.download') || action.endsWith('.export')
}

const generatedReasonKeys = {
  "withdrawal application": "audit.reason.withdrawalApplication",
  "automatic review by saved policy": "audit.reason.automaticReview",
  "administrative permission denied": "audit.reason.permissionDenied",
  "COMPLIANCE_REVIEW_REQUIRED": "audit.reason.complianceReview",
  "Automatic period reservation": "audit.reason.automaticPeriod",
  "Validated automatic source result": "audit.reason.automaticDraw",
  "Mapped source matched frozen period and configured model": "audit.reason.mappedDraw",
  "period already has an immutable archive": "audit.reason.archiveExists",
  "immutable automatic archive captured": "audit.reason.archiveCompleted",
  "automatic archive attempt failed": "audit.reason.archiveFailed",
  "Initial disabled compliance configuration": "audit.reason.initialCompliance",
  "Initial platform compliance configuration": "audit.reason.initialPlatformCompliance",
  "Initial in-app notification template": "audit.reason.initialNotification",
  "Initial platform notification template": "audit.reason.initialPlatformNotification",
  "Initial withdrawal policy": "audit.reason.initialWithdrawal",
  "Automatic member referral allocation": "audit.reason.referralAllocation",
  "automatically register closed saved commission window": "audit.reason.commissionDiscovery",
  "saved window registered; no commission payment": "audit.reason.commissionRegistered",
  "register complete cycle for saved payout mode": "audit.reason.commissionPaymentCreated",
  "approved cycle commission credited to commission available points": "audit.reason.commissionCredited",
  "account for immutable beneficiary total; zero amounts do not create ledger entries": "audit.reason.commissionTargetVerified",
  "all cycle beneficiaries accounted for": "audit.reason.commissionComplete",
  "one beneficiary target committed atomically with its ledger": "audit.reason.commissionPosted",
  "commission posting failed; authorized operator retry required": "audit.reason.commissionFailed",
  "settlement evidence changed; preserve original credits and stop new payments": "audit.reason.commissionInvalidated",
  "final settlement evidence changed; preserve previous run": "audit.reason.cycleInvalidated",
  "all cycle orders reached final states": "audit.reason.cycleFinal",
  "capture immutable closed-window order manifest": "audit.reason.cycleManifest",
  "record exact final-order differential evidence": "audit.reason.cycleEvidence",
  "sum exact per-agent cycle amounts and round once": "audit.reason.cycleEarnings",
  "freeze actual prior commission and replacement calculation; preparation does not move points": "audit.reason.planCreated",
  "replacement evidence changed; preserve the old difference plan without financial execution": "audit.reason.planInvalidated",
  "all immutable per-agent differences verified; this is not compensation authorization or payment": "audit.reason.planReady",
  "prepare bounded immutable differences without ledger or notification postings": "audit.reason.planPage",
  "difference preparation failed; explicit operator retry required": "audit.reason.planFailed",
  "replacement evidence changed before failure recording": "audit.reason.replacementChanged",
  "register exact current difference plan under explicit financial rollout; manual and mixed require fresh approval": "audit.reason.executionCreated",
  "replacement evidence changed; preserve applied differences and the whole-cycle hold": "audit.reason.executionInvalidated",
  "all frozen beneficiary differences committed and verified": "audit.reason.executionComplete",
  "commission available points insufficient; stop entire cycle without using frozen or other sources": "audit.reason.executionPaused",
  "one beneficiary difference and actual awarded net committed atomically": "audit.reason.executionPosted",
  "approved difference posted only to commission available points": "audit.reason.executionCredited",
  "witness exact difference and resulting actual awarded net; zero differences do not post a ledger": "audit.reason.executionVerified",
  "financial difference failed and rolled back; explicit administrator retry required": "audit.reason.executionFailed",
  "COMMISSION_CALCULATION_FAILED": "audit.reason.calculationFailed",
  "COMMISSION_EVIDENCE_INVALID": "audit.reason.calculationEvidenceInvalid",
  "COMMISSION_AMOUNT_OVERFLOW": "audit.reason.commissionOverflow",
  "COMMISSION_DISCOVERY_FAILED": "audit.reason.discoveryFailed",
  "COMMISSION_DISCOVERY_EVIDENCE_INVALID": "audit.reason.discoveryEvidenceInvalid",

  'confirmed configured stake': 'audit.reason.configuredStake',
  'configured loss from reserved original sources': 'audit.reason.configuredLoss',
  'configured winnings including stake': 'audit.reason.configuredPrize',
  'configured push original sources': 'audit.reason.configuredRefund',
  'frozen configured rule settlement': 'audit.reason.configuredSettlement',
  'Owned read-only platform browser verification': 'audit.reason.archiveVerification',
  'automatic report archive capture': 'audit.reason.automaticArchive',
  'discover elapsed report period': 'audit.reason.archivePeriodElapsed',
} as const

/** Localize registered generated reasons while preserving operator-entered notes. */
export function auditReasonLabel(reason: string, locale: 'en' | 'zh-CN'): string {
  if (Object.hasOwn(generatedReasonKeys, reason)) {
    return businessDetailsCatalog[locale][generatedReasonKeys[reason as keyof typeof generatedReasonKeys]]
  }
  return reason || '—'
}
