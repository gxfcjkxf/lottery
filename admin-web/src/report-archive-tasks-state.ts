import { readonly, shallowRef } from "vue";
import type { ReportArchiveTask } from "./report-archive-tasks-api";

export interface ReportArchiveTaskRetryIntent {
  readonly actorId: string;
  readonly brandId: string;
  readonly taskId: string;
  readonly version: number;
  readonly reason: string;
  readonly key: string;
  readonly phase: "unknown" | "conflict" | "acknowledged";
  readonly receipt: Readonly<ReportArchiveTask> | null;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const IDEMPOTENCY_KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const intents = new Map<string, ReportArchiveTaskRetryIntent>();
const generations = new WeakMap<ReportArchiveTaskRetryIntent, number>();
const changes = shallowRef(0);
let generation = 0;

export const reportArchiveTaskIntentChanges = readonly(changes);

function scopeKey(actorId: string, brandId: string, taskId: string): string {
  return JSON.stringify([actorId, brandId, taskId]);
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

function validBase(value: Pick<ReportArchiveTaskRetryIntent, "actorId" | "brandId" | "taskId" | "version" | "reason" | "key">): boolean {
  return UUID.test(value.actorId) && UUID.test(value.brandId) && UUID.test(value.taskId) &&
    Number.isSafeInteger(value.version) && value.version >= 1 && value.version < Number.MAX_SAFE_INTEGER &&
    validReason(value.reason) && IDEMPOTENCY_KEY.test(value.key);
}

function makeIntent(
  actorId: string,
  brandId: string,
  taskId: string,
  version: number,
  reason: string,
  key: string,
  phase: ReportArchiveTaskRetryIntent["phase"],
  receipt: Readonly<ReportArchiveTask> | null = null,
): ReportArchiveTaskRetryIntent {
  const intent = Object.freeze({ actorId, brandId, taskId, version, reason, key, phase, receipt });
  generations.set(intent, generation);
  return intent;
}

export function createReportArchiveTaskRetryIntent(
  actorId: string, brandId: string, taskId: string, version: number, reason: string, key: string,
): ReportArchiveTaskRetryIntent | null {
  const candidate = { actorId, brandId, taskId, version, reason, key };
  return validBase(candidate) ? makeIntent(actorId, brandId, taskId, version, reason, key, "unknown") : null;
}

export function getReportArchiveTaskRetryIntent(actorId: string, brandId: string, taskId: string): ReportArchiveTaskRetryIntent | null {
  if (![actorId, brandId, taskId].every((part) => UUID.test(part))) return null;
  return intents.get(scopeKey(actorId, brandId, taskId)) ?? null;
}

export function getReportArchiveTaskRetryIntents(actorId: string, brandId: string): ReportArchiveTaskRetryIntent[] {
  if (!UUID.test(actorId) || !UUID.test(brandId)) return [];
  return [...intents.values()].filter((intent) => intent.actorId === actorId && intent.brandId === brandId);
}

export function setReportArchiveTaskRetryIntent(intent: ReportArchiveTaskRetryIntent): boolean {
  if (!intent || generations.get(intent) !== generation || !validBase(intent) || !["unknown", "conflict", "acknowledged"].includes(intent.phase)) return false;
  const id = scopeKey(intent.actorId, intent.brandId, intent.taskId);
  const existing = intents.get(id);
  if (existing) return existing.key === intent.key && existing.version === intent.version && existing.reason === intent.reason;
  if ([...intents.values()].some((saved) => saved.actorId === intent.actorId && saved.brandId === intent.brandId)) return false;
  intents.set(id, makeIntent(intent.actorId, intent.brandId, intent.taskId, intent.version, intent.reason, intent.key, intent.phase, intent.receipt));
  changes.value++;
  return true;
}

function transition(intent: ReportArchiveTaskRetryIntent, phase: ReportArchiveTaskRetryIntent["phase"], receipt?: ReportArchiveTask): boolean {
  if (!intent || generations.get(intent) !== generation) return false;
  const id = scopeKey(intent.actorId, intent.brandId, intent.taskId);
  const current = intents.get(id);
  if (!current || current.key !== intent.key || current.version !== intent.version || current.reason !== intent.reason) return false;
  let savedReceipt = phase === "acknowledged" ? current.receipt : null;
  if (phase === "acknowledged") {
    if (!receipt || receipt.id.toLowerCase() !== current.taskId || receipt.brand_id.toLowerCase() !== current.brandId || receipt.version !== current.version + 1 || receipt.state !== "pending") return false;
    const detached: ReportArchiveTask = {
      ...receipt,
      window: Object.freeze({ ...receipt.window }),
    };
    savedReceipt = Object.freeze(detached);
  }
  intents.set(id, makeIntent(current.actorId, current.brandId, current.taskId, current.version, current.reason, current.key, phase, savedReceipt));
  changes.value++;
  return true;
}

export function markReportArchiveTaskRetryConflict(intent: ReportArchiveTaskRetryIntent): boolean { return transition(intent, "conflict"); }
export function markReportArchiveTaskRetryAcknowledged(intent: ReportArchiveTaskRetryIntent, receipt: ReportArchiveTask): boolean {
  return transition(intent, "acknowledged", receipt);
}

export function clearReportArchiveTaskRetryIntent(actorId: string, brandId: string, taskId: string, expectedKey: string): boolean {
  const id = scopeKey(actorId, brandId, taskId);
  const current = intents.get(id);
  if (!current || current.key !== expectedKey) return false;
  intents.delete(id);
  changes.value++;
  return true;
}

export function clearAllReportArchiveTaskRetryIntents(): void {
  intents.clear();
  generation++;
  changes.value++;
}

export function reportArchiveTaskSessionGeneration(): number { return generation; }

export function classifyReportArchiveTaskRetryFailure(status?: number): "unknown" | "conflict" | "definitive" {
  if (status === 409) return "conflict";
  if (status === undefined || status === 0 || status >= 500) return "unknown";
  return "definitive";
}
