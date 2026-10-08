import { readonly, shallowRef } from "vue";
import type { ReportArchivePolicy } from "./report-archive-tasks-api";

export interface ReportArchivePolicyIntent {
  readonly actorId: string;
  readonly brandId: string;
  readonly version: number;
  readonly daily_enabled: boolean;
  readonly monthly_enabled: boolean;
  readonly reason: string;
  readonly key: string;
  readonly phase: "unknown" | "conflict" | "acknowledged";
  readonly receipt: Readonly<ReportArchivePolicy> | null;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const IDEMPOTENCY_KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const POLICY_FIELDS = ["brand_id", "version", "daily_enabled", "monthly_enabled", "daily_start_period", "monthly_start_period", "timezone", "audit_log_id", "updated_at"] as const;
const intents = new Map<string, ReportArchivePolicyIntent>();
const generations = new WeakMap<ReportArchivePolicyIntent, number>();
const changes = shallowRef(0);
let generation = 0;

export const reportArchivePolicyIntentChanges = readonly(changes);

function scopeKey(actorId: string, brandId: string): string { return JSON.stringify([actorId, brandId]); }

function validReason(value: unknown): value is string {
  if (typeof value !== "string" || value.length === 0 || value.trim() !== value || /[\u0000-\u001f\u007f-\u009f]/u.test(value)) return false;
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return new TextEncoder().encode(value).length <= 500;
}

function validBase(value: Pick<ReportArchivePolicyIntent, "actorId" | "brandId" | "version" | "daily_enabled" | "monthly_enabled" | "reason" | "key">): boolean {
  return UUID.test(value.actorId) && UUID.test(value.brandId) &&
    Number.isSafeInteger(value.version) && value.version >= 1 && value.version < Number.MAX_SAFE_INTEGER &&
    typeof value.daily_enabled === "boolean" && typeof value.monthly_enabled === "boolean" &&
    validReason(value.reason) && IDEMPOTENCY_KEY.test(value.key);
}

function record(value: unknown): value is Record<string, unknown> { return value !== null && typeof value === "object" && !Array.isArray(value); }
function exactPolicy(value: Record<string, unknown>): boolean {
  const keys = Object.keys(value).sort(), expected = [...POLICY_FIELDS].sort();
  return keys.length === expected.length && keys.every((key, index) => key === expected[index]);
}
function validPeriod(value: unknown, monthly: boolean): value is string {
  if (typeof value !== "string") return false;
  const match = (monthly ? /^(\d{4})-(\d{2})$/ : /^(\d{4})-(\d{2})-(\d{2})$/).exec(value);
  if (!match) return false;
  const year = Number(match[1]), month = Number(match[2]), day = monthly ? 1 : Number(match[3]);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const lastDay = month === 2 ? leap ? 29 : 28 : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return year >= 1 && year <= 9998 && month >= 1 && month <= 12 && day >= 1 && day <= lastDay;
}
function validPolicyReceipt(value: unknown, intent: ReportArchivePolicyIntent): value is ReportArchivePolicy {
  if (!record(value) || !exactPolicy(value) || typeof value.brand_id !== "string" || !UUID.test(value.brand_id) ||
    value.brand_id.toLowerCase() !== intent.brandId.toLowerCase() || !Number.isSafeInteger(value.version) ||
    value.version !== intent.version + 1 || typeof value.daily_enabled !== "boolean" || value.daily_enabled !== intent.daily_enabled ||
    typeof value.monthly_enabled !== "boolean" || value.monthly_enabled !== intent.monthly_enabled ||
    (value.daily_start_period !== null && !validPeriod(value.daily_start_period, false)) ||
    (value.monthly_start_period !== null && !validPeriod(value.monthly_start_period, true)) ||
    (value.daily_enabled && value.daily_start_period === null) || (value.monthly_enabled && value.monthly_start_period === null) ||
    typeof value.timezone !== "string" || !value.timezone || value.timezone === "Local" || value.timezone.length > 100 ||
    typeof value.audit_log_id !== "string" || !UUID.test(value.audit_log_id) ||
    typeof value.updated_at !== "string" || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value.updated_at) ||
    !Number.isFinite(Date.parse(value.updated_at))) return false;
  try { new Intl.DateTimeFormat("en", { timeZone: value.timezone }); } catch { return false; }
  return true;
}

function makeIntent(
  actorId: string, brandId: string, version: number, daily_enabled: boolean, monthly_enabled: boolean,
  reason: string, key: string, phase: ReportArchivePolicyIntent["phase"], receipt: Readonly<ReportArchivePolicy> | null = null,
): ReportArchivePolicyIntent {
  const intent = Object.freeze({ actorId, brandId, version, daily_enabled, monthly_enabled, reason, key, phase, receipt });
  generations.set(intent, generation);
  return intent;
}

export function createReportArchivePolicyIntent(
  actorId: string, brandId: string, version: number, daily_enabled: boolean, monthly_enabled: boolean, reason: string, key: string,
): ReportArchivePolicyIntent | null {
  const candidate = { actorId, brandId, version, daily_enabled, monthly_enabled, reason, key };
  return validBase(candidate) ? makeIntent(actorId, brandId, version, daily_enabled, monthly_enabled, reason, key, "unknown") : null;
}

export function getReportArchivePolicyIntent(actorId: string, brandId: string): ReportArchivePolicyIntent | null {
  if (!UUID.test(actorId) || !UUID.test(brandId)) return null;
  return intents.get(scopeKey(actorId, brandId)) ?? null;
}

export function getReportArchivePolicyIntents(actorId: string): ReportArchivePolicyIntent[] {
  if (!UUID.test(actorId)) return [];
  return [...intents.values()].filter(intent => intent.actorId === actorId);
}

export function setReportArchivePolicyIntent(intent: ReportArchivePolicyIntent): boolean {
  if (!intent || generations.get(intent) !== generation || !validBase(intent) || !["unknown", "conflict", "acknowledged"].includes(intent.phase)) return false;
  const id = scopeKey(intent.actorId, intent.brandId), existing = intents.get(id);
  if (existing) return existing.key === intent.key && existing.version === intent.version && existing.reason === intent.reason &&
    existing.daily_enabled === intent.daily_enabled && existing.monthly_enabled === intent.monthly_enabled;
  intents.set(id, makeIntent(intent.actorId, intent.brandId, intent.version, intent.daily_enabled, intent.monthly_enabled, intent.reason, intent.key, intent.phase, intent.receipt));
  changes.value++;
  return true;
}

function transition(intent: ReportArchivePolicyIntent, phase: ReportArchivePolicyIntent["phase"], receipt?: ReportArchivePolicy): boolean {
  if (!intent || generations.get(intent) !== generation) return false;
  const id = scopeKey(intent.actorId, intent.brandId), current = intents.get(id);
  if (!current || current.key !== intent.key || current.version !== intent.version || current.reason !== intent.reason ||
    current.daily_enabled !== intent.daily_enabled || current.monthly_enabled !== intent.monthly_enabled) return false;
  if (phase === "conflict" && current.phase === "acknowledged") return false;
  let savedReceipt: Readonly<ReportArchivePolicy> | null = phase === "acknowledged" ? current.receipt : null;
  if (phase === "acknowledged") {
    if (!validPolicyReceipt(receipt, current)) return false;
    savedReceipt = Object.freeze({ ...receipt });
  }
  intents.set(id, makeIntent(current.actorId, current.brandId, current.version, current.daily_enabled, current.monthly_enabled, current.reason, current.key, phase, savedReceipt));
  changes.value++;
  return true;
}

export function markReportArchivePolicyConflict(intent: ReportArchivePolicyIntent): boolean { return transition(intent, "conflict"); }
export function markReportArchivePolicyAcknowledged(intent: ReportArchivePolicyIntent, receipt: ReportArchivePolicy): boolean {
  return transition(intent, "acknowledged", receipt);
}

export function clearReportArchivePolicyIntent(actorId: string, brandId: string, expectedKey: string): boolean {
  const id = scopeKey(actorId, brandId), current = intents.get(id);
  if (!current || current.key !== expectedKey) return false;
  intents.delete(id);
  changes.value++;
  return true;
}

export function clearAllReportArchivePolicyIntents(): void {
  intents.clear();
  generation++;
  changes.value++;
}

export function reportArchivePolicySessionGeneration(): number { return generation; }

export function classifyReportArchivePolicyFailure(status?: number): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  return "definitive";
}
