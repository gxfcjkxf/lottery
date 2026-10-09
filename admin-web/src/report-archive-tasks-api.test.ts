import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { createReportArchiveTasksApi, reportArchiveTasksPermissions, type ReportArchivePolicy, type ReportArchiveTask } from "./report-archive-tasks-api";

const brand = "11111111-1111-4111-8111-111111111111";
const taskId = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const audit1 = "44444444-4444-4444-8444-444444444444";
const audit2 = "55555555-5555-4555-8555-555555555555";
const archiveId = "66666666-6666-4666-8666-666666666666";
const timestamp = "2026-10-07T00:00:00.123456789Z";
const policy: ReportArchivePolicy = {
  brand_id: brand, version: 1, daily_enabled: false, monthly_enabled: false, daily_start_period: null,
  monthly_start_period: null, timezone: "Asia/Singapore", audit_log_id: null, updated_at: timestamp,
};
function task(overrides: Partial<ReportArchiveTask> = {}): ReportArchiveTask {
  return {
    id: taskId, brand_id: brand, policy_version: 1,
    window: { kind: "daily", period_key: "2026-10-06", timezone: "Asia/Singapore", from: "2026-10-05T16:00:00Z", to: "2026-10-06T16:00:00Z" },
    state: "pending", version: 1, attempt_count: 0, archive_id: null, last_error_code: null,
    creation_audit_log_id: audit1, last_audit_log_id: audit1, created_at: timestamp, updated_at: timestamp, ...overrides,
  };
}
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data, request_id: "test" }), { status });
const fetcher = (...data: unknown[]) => vi.fn<typeof fetch>().mockImplementation(async () => ok(data.shift()));

describe("report archive tasks API", () => {
  it("computes view and retry grants within brand membership and denies super admins", () => {
    const base = { id: actor, super_admin: false, brand_ids: [brand], permissions: [] } satisfies AdminAccount;
    expect(reportArchiveTasksPermissions({ ...base, permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_task.retry.brand"] } }, brand)).toEqual({ view: true, retry: true });
    expect(reportArchiveTasksPermissions({ ...base, brand_ids: [], permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_task.retry.brand"] } }, brand)).toEqual({ view: false, retry: false });
    expect(reportArchiveTasksPermissions({ ...base, super_admin: true, permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_task.retry.brand"] }, platform_permissions: ["report_archive.view.platform"] }, brand)).toEqual({ view: false, retry: false });
    expect(reportArchiveTasksPermissions({ ...base, platform_permissions: ["report_archive.view.platform"] }, brand)).toEqual({ view: false, retry: false });
    expect(reportArchiveTasksPermissions({ ...base, platform_permissions: ["report_archive.view.platform"], permissions_by_brand: { [brand]: ["report_archive_task.retry.brand"] } }, brand)).toEqual({ view: false, retry: false });
  });

  it("reads policy, list, and detail with strict 200, explicit brand header, and exact response scope", async () => {
    const f = fetcher(policy, { brand_id: brand, items: [task()], total_count: "1", limit: 20, offset: 0 }, task());
    const api = createReportArchiveTasksApi(f);
    await expect(api.policy(brand)).resolves.toEqual(policy);
    await expect(api.list(brand)).resolves.toMatchObject({ total_count: "1" });
    await expect(api.read(brand, taskId)).resolves.toMatchObject({ id: taskId });
    expect(f.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/report-archive-policy",
      "/api/v1/admin/report-archive-tasks?limit=20&offset=0",
      `/api/v1/admin/report-archive-tasks/${taskId}`,
    ]);
    for (const [, init] of f.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("sends retry identity and accepts only a version+1 pending acknowledgement", async () => {
    const saved = task({ version: 3, attempt_count: 1, last_audit_log_id: audit2 });
    const f = vi.fn<typeof fetch>().mockResolvedValue(ok(saved));
    await expect(createReportArchiveTasksApi(f).retry(brand, taskId, { version: 2, reason: "retry after investigation" }, "retry-key-0001", actor)).resolves.toEqual(saved);
    expect(f).toHaveBeenCalledOnce();
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe(`/api/v1/admin/report-archive-tasks/${taskId}/retry`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ version: 2, reason: "retry after investigation" });
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("retry-key-0001");
    expect(new Headers(init?.headers).get("X-Report-Archive-Actor-ID")).toBe(actor);
    for (const bad of [task({ version: 1 }), task({ ...saved, version: 4, state: "completed", archive_id: archiveId, attempt_count: 2 })]) {
      await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(bad))).retry(brand, taskId, { version: 2, reason: "investigated" }, "retry-key-0001", actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    }
  });

  it("keeps conflict codes and treats network or ambiguous server results as unknown writes", async () => {
    const conflict = new Response(JSON.stringify({ success: false, error: { code: "VERSION_CONFLICT", message: "stale" } }), { status: 409 });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(conflict)).retry(brand, taskId, { version: 1, reason: "retry now" }, "retry-key-0001", actor)).rejects.toMatchObject({ status: 409, code: "VERSION_CONFLICT" });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).retry(brand, taskId, { version: 1, reason: "retry now" }, "retry-key-0001", actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(new Response("oops", { status: 200 }))).retry(brand, taskId, { version: 1, reason: "retry now" }, "retry-key-0001", actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
  });

  it("rejects unsafe input and malformed closed DTOs before trusting them", async () => {
    const f = vi.fn<typeof fetch>().mockResolvedValue(ok(policy));
    const api = createReportArchiveTasksApi(f);
    for (const [limit, offset] of [[0, 0], [101, 0], [20, 1_000_001], [1.5, 0]]) await expect(Promise.resolve().then(() => api.list(brand, limit, offset))).rejects.toMatchObject({ code: "INVALID_INPUT" });
    for (const input of [
      { version: Number.MAX_SAFE_INTEGER, reason: "reason" }, { version: 0, reason: "reason" },
      { version: 1, reason: " padded " }, { version: 1, reason: "" }, { version: 1, reason: "bad\u0085" },
      { version: 1, reason: "é".repeat(251) }, { version: 1, reason: "bad\uD800" }, { version: 1, reason: "good", extra: true },
    ]) await expect(Promise.resolve().then(() => api.retry(brand, taskId, input as never, "retry-key-0001", actor))).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(f).not.toHaveBeenCalled();
    for (const malformed of [
      { ...policy, unexpected: true }, { ...policy, version: Number.MAX_SAFE_INTEGER + 1 }, { ...policy, daily_enabled: true },
      { ...policy, audit_log_id: audit1 },
    ]) await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(malformed))).policy(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    for (const malformed of [
      task({ brand_id: "77777777-7777-4777-8777-777777777777" }),
      task({ window: { ...task().window, to: "2026-10-05T16:00:00Z" } }),
      task({ state: "completed", archive_id: null, last_error_code: null, attempt_count: 1 }),
      task({ state: "skipped", archive_id: null, last_error_code: null, attempt_count: 1, version: 2 }),
      task({ state: "failed", archive_id: archiveId, last_error_code: "ARCHIVE_FAILED", attempt_count: 1, version: 2 }),
      task({ state: "failed", archive_id: null, last_error_code: "FAILED", attempt_count: 1, version: 2 }),
      task({ attempt_count: Number.MAX_SAFE_INTEGER + 1 }),
    ]) await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(malformed))).read(brand, taskId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(Promise.resolve().then(() => createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(task()))).read(brand, "not-a-uuid"))).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });

  it("compares exact total_count strings without Number and enforces unique descending ordering", async () => {
    const hugePage = { brand_id: brand, items: [task()], total_count: "900719925474099312345", limit: 1, offset: 0 };
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(hugePage))).list(brand, 1, 0)).resolves.toMatchObject({ total_count: hugePage.total_count });
    const duplicate = { ...hugePage, limit: 2, items: [
      task({ created_at: "2026-10-07T00:01:00Z", updated_at: "2026-10-07T00:01:00Z" }),
      task({ id: taskId.toUpperCase() }),
    ] };
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(duplicate))).list(brand, 2, 0)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongOrder = { ...hugePage, limit: 2, items: [task({ id: "11111111-1111-4111-8111-111111111112" }), task()] };
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(wrongOrder))).list(brand, 2, 0)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const missingRows = { brand_id: brand, items: [], total_count: "10", limit: 20, offset: 0 };
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(missingRows))).list(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const beyondEnd = { ...missingRows, offset: 20 };
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(beyondEnd))).list(brand, 20, 20)).resolves.toMatchObject({ items: [] });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(policy, 201))).policy(brand)).rejects.toBeInstanceOf(AdminApiError);
  });

  it("accepts frozen window instants with alternate offsets without recalculating timezone boundaries", async () => {
    const spring = task({ created_at: "2026-03-09T05:00:00Z", updated_at: "2026-03-09T05:00:00Z", window: {
      kind: "daily", period_key: "2026-03-08", timezone: "America/New_York", from: "2026-03-08T00:00:00-05:00", to: "2026-03-09T00:00:00-04:00",
    } });
    const frozen = task({ created_at: "2026-03-09T06:00:00Z", updated_at: "2026-03-09T06:00:00Z", window: {
      kind: "daily", period_key: "2026-03-08", timezone: "America/New_York", from: "2026-03-08T00:30:00-05:00", to: "2026-03-09T01:00:00-04:00",
    } });
    const leapMonth = task({ created_at: "2024-03-01T00:00:00Z", updated_at: "2024-03-01T00:00:00Z", window: {
      kind: "monthly", period_key: "2024-02", timezone: "Asia/Singapore", from: "2024-01-31T16:00:00Z", to: "2024-02-29T16:00:00Z",
    } });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(spring))).read(brand, taskId)).resolves.toMatchObject({ window: spring.window });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(frozen))).read(brand, taskId)).resolves.toMatchObject({ window: frozen.window });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(leapMonth))).read(brand, taskId)).resolves.toMatchObject({ window: leapMonth.window });
  });

  it("accepts skipped archives with archive evidence and requires the fixed failure code", async () => {
    const skipped = task({ state: "skipped", version: 2, attempt_count: 1, archive_id: archiveId, last_error_code: null });
    const failed = task({ state: "failed", version: 2, attempt_count: 1, archive_id: null, last_error_code: "ARCHIVE_FAILED" });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(skipped))).read(brand, taskId)).resolves.toMatchObject({ state: "skipped", archive_id: archiveId });
    await expect(createReportArchiveTasksApi(vi.fn<typeof fetch>().mockResolvedValue(ok(failed))).read(brand, taskId)).resolves.toMatchObject({ state: "failed", last_error_code: "ARCHIVE_FAILED" });
  });
});
