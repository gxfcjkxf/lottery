import type { ReportArchiveCreateInput } from "./report-archives-api";
import { readonly, shallowRef } from "vue";

export interface ArchiveIntent {
  readonly actorId: string;
  readonly brandId: string;
  readonly body: Readonly<ReportArchiveCreateInput>;
  readonly key: string;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const IDEMPOTENCY_KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const DAY = /^(\d{4})-(\d{2})-(\d{2})$/;
const MONTH = /^(\d{4})-(\d{2})$/;
const intents = new Map<string, ArchiveIntent>();
const intentGenerations = new WeakMap<ArchiveIntent, number>();
const conflicts = new Map<string, string>();
let generation = 0;
const changes = shallowRef(0);
export const archiveIntentChanges = readonly(changes);

function scopeKey(actorId: string, brandId: string): string {
  return JSON.stringify([actorId, brandId]);
}

function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID.test(value);
}

function validDay(value: string): boolean {
  const match = DAY.exec(value);
  if (!match) return false;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (year < 1 || year > 9998 || month < 1 || month > 12) return false;
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const lastDay = month === 2 ? (leap ? 29 : 28) : [4, 6, 9, 11].includes(month) ? 30 : 31;
  return day >= 1 && day <= lastDay;
}

function validPeriod(kind: unknown, periodKey: unknown): boolean {
  if (typeof periodKey !== "string") return false;
  if (kind === "daily") return validDay(periodKey);
  if (kind !== "monthly") return false;
  const match = MONTH.exec(periodKey);
  return Boolean(match && Number(match[1]) >= 1 && Number(match[1]) <= 9998 && Number(match[2]) >= 1 && Number(match[2]) <= 12);
}

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

function validBody(body: unknown): body is ReportArchiveCreateInput {
  if (body === null || typeof body !== "object" || Array.isArray(body)) return false;
  const value = body as Record<string, unknown>;
  const keys = Object.keys(value).sort();
  return keys.length === 4 && keys[0] === "expected_revision" && keys[1] === "kind" && keys[2] === "period_key" && keys[3] === "reason" &&
    validPeriod(value.kind, value.period_key) && Number.isSafeInteger(value.expected_revision) &&
    (value.expected_revision as number) >= 0 && (value.expected_revision as number) < Number.MAX_SAFE_INTEGER && validReason(value.reason);
}

function copyIntent(actorId: string, brandId: string, body: ReportArchiveCreateInput, key: string): ArchiveIntent {
  const frozenBody = Object.freeze({
    kind: body.kind,
    period_key: body.period_key,
    expected_revision: body.expected_revision,
    reason: body.reason,
  });
  const intent = Object.freeze({ actorId, brandId, body: frozenBody, key });
  intentGenerations.set(intent, generation);
  return intent;
}

export function createArchiveIntent(actorId: string, brandId: string, body: ReportArchiveCreateInput, key: string): ArchiveIntent | null {
  if (!validUuid(actorId) || !validUuid(brandId) || !validBody(body) || typeof key !== "string" || !IDEMPOTENCY_KEY.test(key)) return null;
  return copyIntent(actorId, brandId, body, key);
}

export function getArchiveIntent(actorId: string, brandId: string): ArchiveIntent | null {
  if (!validUuid(actorId) || !validUuid(brandId)) return null;
  return intents.get(scopeKey(actorId, brandId)) ?? null;
}

export function setArchiveIntent(intent: ArchiveIntent): boolean {
  if (!intent || intentGenerations.get(intent) !== generation || !validUuid(intent.actorId) || !validUuid(intent.brandId) || !validBody(intent.body) ||
    typeof intent.key !== "string" || !IDEMPOTENCY_KEY.test(intent.key)) return false;
  const id = scopeKey(intent.actorId, intent.brandId);
  const existing = intents.get(id);
  if (existing) return existing.key === intent.key && existing.body.kind === intent.body.kind &&
    existing.body.period_key === intent.body.period_key && existing.body.expected_revision === intent.body.expected_revision &&
    existing.body.reason === intent.body.reason;
  const saved = copyIntent(intent.actorId, intent.brandId, intent.body, intent.key);
  intents.set(id, saved);
  changes.value++;
  return true;
}

export function clearArchiveIntent(actorId: string, brandId: string, expectedKey?: string): boolean {
  if (!validUuid(actorId) || !validUuid(brandId)) return false;
  const id = scopeKey(actorId, brandId);
  const current = intents.get(id);
  if (!current || expectedKey !== undefined && current.key !== expectedKey) return false;
  intents.delete(id);
  conflicts.delete(id);
  changes.value++;
  return true;
}

export function markArchiveConflict(intent: ArchiveIntent): boolean {
  if (!intent || intentGenerations.get(intent) !== generation) return false;
  const id = scopeKey(intent.actorId, intent.brandId);
  const current = intents.get(id);
  if (!current || current.key !== intent.key) return false;
  conflicts.set(id, intent.key);
  changes.value++;
  return true;
}

export function isArchiveConflict(intent: ArchiveIntent): boolean {
  if (!intent || intentGenerations.get(intent) !== generation) return false;
  const id = scopeKey(intent.actorId, intent.brandId);
  return intents.get(id)?.key === intent.key && conflicts.get(id) === intent.key;
}

export function clearAllArchiveIntents(): void {
  intents.clear();
  conflicts.clear();
  generation++;
  changes.value++;
}

export function archiveSessionGeneration(): number {
  return generation;
}

export function classifyArchiveWriteFailure(status?: number): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  return "definitive";
}
