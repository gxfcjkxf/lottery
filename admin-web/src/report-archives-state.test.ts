import { afterEach, describe, expect, it } from "vitest";
import {
  archiveSessionGeneration,
  classifyArchiveWriteFailure,
  clearAllArchiveIntents,
  clearArchiveIntent,
  createArchiveIntent,
  getArchiveIntent,
  isArchiveConflict,
  markArchiveConflict,
  setArchiveIntent,
} from "./report-archives-state";
import type { ReportArchiveCreateInput } from "./report-archives-api";

const actor = "00000000-0000-4000-8000-000000000001";
const otherActor = "00000000-0000-4000-8000-000000000003";
const brand = "00000000-0000-4000-8000-000000000002";
const otherBrand = "00000000-0000-4000-8000-000000000004";
const body: ReportArchiveCreateInput = { kind: "daily", period_key: "2024-02-29", expected_revision: 0, reason: "Approved month end ✅" };

afterEach(() => clearAllArchiveIntents());

describe("report archive intent state", () => {
  it("stores a detached, frozen create body in an actor and brand scope", () => {
    const mutable = { ...body };
    const intent = createArchiveIntent(actor, brand, mutable, "archive-create-key-001");
    expect(intent).not.toBeNull();
    expect(setArchiveIntent(intent!)).toBe(true);
    mutable.reason = "Changed after confirmation";
    const saved = getArchiveIntent(actor, brand);
    expect(saved?.body).toEqual(body);
    expect(Object.isFrozen(saved)).toBe(true);
    expect(Object.isFrozen(saved?.body)).toBe(true);
    expect(getArchiveIntent(otherActor, brand)).toBeNull();
    expect(getArchiveIntent(actor, otherBrand)).toBeNull();
  });

  it("keeps separate intents across actors and brands, but one unresolved key per scope", () => {
    const first = createArchiveIntent(actor, brand, body, "archive-create-key-001")!;
    const differentBody = { ...body, kind: "monthly" as const, period_key: "2024-02" };
    const replacement = createArchiveIntent(actor, brand, differentBody, "archive-create-key-002")!;
    const separateBrand = createArchiveIntent(actor, otherBrand, body, "archive-create-key-003")!;
    const separateActor = createArchiveIntent(otherActor, brand, body, "archive-create-key-004")!;
    expect(setArchiveIntent(first)).toBe(true);
    expect(setArchiveIntent(replacement)).toBe(false);
    expect(setArchiveIntent(separateBrand)).toBe(true);
    expect(setArchiveIntent(separateActor)).toBe(true);
    expect(getArchiveIntent(actor, brand)?.key).toBe(first.key);
  });

  it("allows a late conflict for the retained matching intent and protects replacement keys", () => {
    const intent = createArchiveIntent(actor, brand, body, "archive-create-key-001")!;
    expect(setArchiveIntent(intent)).toBe(true);
    expect(markArchiveConflict(intent)).toBe(true);
    expect(isArchiveConflict(intent)).toBe(true);
    expect(markArchiveConflict({ ...intent, key: "archive-create-key-999" })).toBe(false);
    expect(clearArchiveIntent(actor, brand, intent.key)).toBe(true);
    const replacement = createArchiveIntent(actor, brand, body, "archive-create-key-002")!;
    expect(setArchiveIntent(replacement)).toBe(true);
    expect(markArchiveConflict(intent)).toBe(false);
    expect(isArchiveConflict(replacement)).toBe(false);
  });

  it("guards clears by key and invalidates old callbacks when the session generation changes", () => {
    const oldGeneration = archiveSessionGeneration();
    const old = createArchiveIntent(actor, brand, body, "archive-create-key-001")!;
    setArchiveIntent(old);
    expect(clearArchiveIntent(actor, brand, "wrong-create-key-001")).toBe(false);
    expect(getArchiveIntent(actor, brand)?.key).toBe(old.key);
    clearAllArchiveIntents();
    expect(archiveSessionGeneration()).toBe(oldGeneration + 1);
    const current = createArchiveIntent(actor, brand, body, old.key)!;
    expect(setArchiveIntent(current)).toBe(true);
    expect(markArchiveConflict(old)).toBe(false);
    expect(isArchiveConflict(current)).toBe(false);
    expect(clearArchiveIntent(actor, brand, old.key)).toBe(true);
  });

  it("classifies ambiguous, conflict, and definitive failures", () => {
    expect(classifyArchiveWriteFailure()).toBe("unknown");
    expect(classifyArchiveWriteFailure(0)).toBe("unknown");
    expect(classifyArchiveWriteFailure(503)).toBe("unknown");
    expect(classifyArchiveWriteFailure(409)).toBe("conflict");
    expect(classifyArchiveWriteFailure(400)).toBe("definitive");
    expect(classifyArchiveWriteFailure(403)).toBe("definitive");
  });

  it("accepts canonical UUIDs, exact bodies, valid calendar boundaries, and UTF-8 reasons only", () => {
    expect(createArchiveIntent("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", brand, body, "archive-create-key-001")).toBeNull();
    expect(createArchiveIntent("00000000-0000-0000-8000-000000000001", brand, body, "archive-create-key-001")).toBeNull();
    expect(createArchiveIntent(actor, brand, { ...body, unexpected: true } as ReportArchiveCreateInput, "archive-create-key-001")).toBeNull();
    for (const period_key of ["0000-01-01", "9999-01-01", "2023-02-29", "2024-02-30", "2024-2-01", "2024-02-01 "]) {
      expect(createArchiveIntent(actor, brand, { ...body, period_key }, "archive-create-key-001")).toBeNull();
    }
    for (const period_key of ["0001-01-01", "9998-12-31", "2000-02-29"]) {
      expect(createArchiveIntent(actor, brand, { ...body, period_key }, "archive-create-key-001")).not.toBeNull();
    }
    expect(createArchiveIntent(actor, brand, { ...body, kind: "monthly", period_key: "0001-01" }, "archive-create-key-001")).not.toBeNull();
    for (const period_key of ["0000-01", "9999-01", "2024-00", "2024-13", "2024-1"]) {
      expect(createArchiveIntent(actor, brand, { ...body, kind: "monthly", period_key }, "archive-create-key-001")).toBeNull();
    }
    for (const reason of ["", " padded ", "line\nbreak", "tab\there", "nul\u0000", "control\u0085", "lone\ud800", "✅".repeat(167)]) {
      expect(createArchiveIntent(actor, brand, { ...body, reason }, "archive-create-key-001")).toBeNull();
    }
    expect(createArchiveIntent(actor, brand, { ...body, reason: "✅".repeat(166) }, "archive-create-key-001")).not.toBeNull();
    expect(createArchiveIntent(actor, brand, { ...body, expected_revision: Number.MAX_SAFE_INTEGER }, "archive-create-key-001")).toBeNull();
  });

  it("rejects malformed keys and preserves the first unresolved key", () => {
    for (const key of ["short", "bad key 123", "key/with/slash", "x".repeat(129)]) {
      expect(createArchiveIntent(actor, brand, body, key)).toBeNull();
    }
    const intent = createArchiveIntent(actor, brand, body, "archive-create-key-001")!;
    expect(setArchiveIntent(intent)).toBe(true);
    expect(setArchiveIntent(createArchiveIntent(actor, brand, body, "archive-create-key-002")!)).toBe(false);
    expect(clearArchiveIntent(actor, brand, "archive-create-key-002")).toBe(false);
    expect(clearArchiveIntent(actor, brand, intent.key)).toBe(true);
    expect(clearArchiveIntent(actor, brand)).toBe(false);
  });
});
