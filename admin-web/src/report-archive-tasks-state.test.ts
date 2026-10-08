import { afterEach, describe, expect, it } from "vitest";
import type { ReportArchiveTask } from "./report-archive-tasks-api";
import {
  clearAllReportArchiveTaskRetryIntents,
  clearReportArchiveTaskRetryIntent,
  classifyReportArchiveTaskRetryFailure,
  createReportArchiveTaskRetryIntent,
  getReportArchiveTaskRetryIntent,
  getReportArchiveTaskRetryIntents,
  markReportArchiveTaskRetryAcknowledged,
  markReportArchiveTaskRetryConflict,
  reportArchiveTaskSessionGeneration,
  setReportArchiveTaskRetryIntent,
} from "./report-archive-tasks-state";

const actor = "33333333-3333-4333-8333-333333333333";
const brand = "11111111-1111-4111-8111-111111111111";
const task = "44444444-4444-4444-8444-444444444444";
const anotherTask = "66666666-6666-4666-8666-666666666666";
const make = (taskId = task, reason = "Retry after checking source") => createReportArchiveTaskRetryIntent(actor, brand, taskId, 4, reason, "archive-retry-key-001")!;
const receipt = { id: task, brand_id: brand, policy_version: 1, window: { kind: "daily", period_key: "2026-10-01", timezone: "UTC", from: "2026-10-01T00:00:00Z", to: "2026-10-02T00:00:00Z" }, state: "pending", version: 5, attempt_count: 2, archive_id: null, last_error_code: null, creation_audit_log_id: "55555555-5555-4555-8555-555555555555", last_audit_log_id: "77777777-7777-4777-8777-777777777777", created_at: "2026-10-01T00:00:00Z", updated_at: "2026-10-03T00:00:00Z" } as ReportArchiveTask;

afterEach(() => clearAllReportArchiveTaskRetryIntents());

describe("report archive task retry intent state", () => {
  it("freezes original identity, version, reason and key in session memory", () => {
    const intent = make();
    expect(setReportArchiveTaskRetryIntent(intent)).toBe(true);
    expect(Object.isFrozen(getReportArchiveTaskRetryIntent(actor, brand, task))).toBe(true);
    expect(getReportArchiveTaskRetryIntent(actor, brand, task)).toMatchObject({ actorId: actor, brandId: brand, taskId: task, version: 4, reason: "Retry after checking source", key: "archive-retry-key-001", phase: "unknown" });
    expect(getReportArchiveTaskRetryIntents(actor, brand)).toHaveLength(1);
  });

  it("locks one retry across tasks for an account and brand until explicitly released", () => {
    expect(setReportArchiveTaskRetryIntent(make())).toBe(true);
    expect(setReportArchiveTaskRetryIntent(make(anotherTask))).toBe(false);
    expect(getReportArchiveTaskRetryIntent(actor, brand, task)?.key).toBe("archive-retry-key-001");
  });

  it("retains the original key through conflict and acknowledged states", () => {
    const intent = make(); setReportArchiveTaskRetryIntent(intent);
    expect(markReportArchiveTaskRetryConflict(intent)).toBe(true);
    expect(getReportArchiveTaskRetryIntent(actor, brand, task)?.phase).toBe("conflict");
    const conflict = getReportArchiveTaskRetryIntent(actor, brand, task)!;
    const mutableReceipt = { ...receipt, window: { ...receipt.window } };
    expect(markReportArchiveTaskRetryAcknowledged(conflict, mutableReceipt)).toBe(true);
    mutableReceipt.state = "completed";
    const acknowledged = getReportArchiveTaskRetryIntent(actor, brand, task)!;
    expect(acknowledged).toMatchObject({ key: intent.key, phase: "acknowledged", receipt: { state: "pending", version: 5 } });
    expect(Object.isFrozen(acknowledged.receipt)).toBe(true);
    expect(Object.isFrozen(acknowledged.receipt?.window)).toBe(true);
    expect(clearReportArchiveTaskRetryIntent(actor, brand, task, "wrong-key-0001")).toBe(false);
    expect(clearReportArchiveTaskRetryIntent(actor, brand, task, intent.key)).toBe(true);
  });

  it("isolates callbacks across cleared session generations", () => {
    const before = reportArchiveTaskSessionGeneration(), old = make(); setReportArchiveTaskRetryIntent(old);
    clearAllReportArchiveTaskRetryIntents();
    expect(reportArchiveTaskSessionGeneration()).toBe(before + 1);
    const current = make(anotherTask); expect(setReportArchiveTaskRetryIntent(current)).toBe(true);
    expect(markReportArchiveTaskRetryConflict(old)).toBe(false);
    expect(getReportArchiveTaskRetryIntent(actor, brand, anotherTask)?.phase).toBe("unknown");
  });

  it("validates identifiers, versions, reason bytes and idempotency keys", () => {
    expect(createReportArchiveTaskRetryIntent("bad", brand, task, 1, "reason", "archive-retry-key-001")).toBeNull();
    expect(createReportArchiveTaskRetryIntent(actor, brand, task, -1, "reason", "archive-retry-key-001")).toBeNull();
    expect(createReportArchiveTaskRetryIntent(actor, brand, task, 0, "reason", "archive-retry-key-001")).toBeNull();
    expect(createReportArchiveTaskRetryIntent(actor, brand, task, 1, " padded ", "archive-retry-key-001")).toBeNull();
    expect(createReportArchiveTaskRetryIntent(actor, brand, task, 1, "✅".repeat(167), "archive-retry-key-001")).toBeNull();
    expect(createReportArchiveTaskRetryIntent(actor, brand, task, 1, "valid", "short")).toBeNull();
  });

  it("classifies unknown, conflict and definitive outcomes", () => {
    expect(classifyReportArchiveTaskRetryFailure()).toBe("unknown");
    expect(classifyReportArchiveTaskRetryFailure(0)).toBe("unknown");
    expect(classifyReportArchiveTaskRetryFailure(503)).toBe("unknown");
    expect(classifyReportArchiveTaskRetryFailure(409)).toBe("conflict");
    expect(classifyReportArchiveTaskRetryFailure(403)).toBe("definitive");
  });
});
