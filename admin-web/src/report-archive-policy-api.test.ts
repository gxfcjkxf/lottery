import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createReportArchivePolicyApi,
  reportArchivePolicyPermissions,
  type UpdateReportArchivePolicyInput,
} from "./report-archive-policy-api";
import type { ReportArchivePolicy } from "./report-archive-tasks-api";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const audit = "33333333-3333-4333-8333-333333333333";
const key = "archive-policy-key-01";
const input: UpdateReportArchivePolicyInput = {
  version: 4, daily_enabled: true, monthly_enabled: false, reason: "Enable daily archives",
};
const policy: ReportArchivePolicy = {
  brand_id: brand, version: 5, daily_enabled: true, monthly_enabled: false,
  daily_start_period: "2026-10-08", monthly_start_period: "2026-09",
  timezone: "Asia/Singapore", audit_log_id: audit, updated_at: "2026-10-08T01:02:03.123456789Z",
};
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data, request_id: "test" }), { status });
const failure = (status: number) => new Response(JSON.stringify({ success: false, error: { code: `ERROR_${status}`, message: "request failed" } }), { status });

describe("report archive policy API", () => {
  it("sends the closed PUT body and authenticated scope headers and validates the acknowledgement", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(policy));
    await expect(createReportArchivePolicyApi(fetcher).update(brand, input, key, actor)).resolves.toEqual(policy);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(url).toBe("/api/v1/admin/report-archive-policy");
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("same-origin");
    expect(JSON.parse(String(init?.body))).toEqual(input);
    expect(JSON.parse(String(init?.body))).not.toHaveProperty("daily_start_period");
    expect(JSON.parse(String(init?.body))).not.toHaveProperty("timezone");
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe(key);
    expect(headers.get("X-Report-Archive-Actor-ID")).toBe(actor);
  });

  it("accepts server-derived enabled starts and disabled starts that are null or retained", async () => {
    const disabled = { ...policy, daily_enabled: false, monthly_enabled: false, daily_start_period: null, monthly_start_period: null };
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(disabled))).update(
      brand, { version: 4, daily_enabled: false, monthly_enabled: false, reason: "Disable archiving" }, key, actor,
    )).resolves.toEqual(disabled);
    const retained = { ...disabled, daily_start_period: "2026-10-01", monthly_start_period: "2026-10" };
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(retained))).update(
      brand, { version: 4, daily_enabled: false, monthly_enabled: false, reason: "Keep saved starts" }, key, actor,
    )).resolves.toEqual(retained);
  });

  it("freezes the validated request snapshot across an in-flight response", async () => {
    let complete!: (response: Response) => void;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(() => new Promise(resolve => { complete = resolve; }));
    const mutable: UpdateReportArchivePolicyInput = { ...input };
    const pending = createReportArchivePolicyApi(fetcher).update(brand, mutable, key, actor);
    mutable.version = 9;
    mutable.daily_enabled = false;
    mutable.monthly_enabled = true;
    mutable.reason = "edited while request is pending";
    complete(ok(policy));
    await expect(pending).resolves.toEqual(policy);
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual(input);
  });

  it("rejects extra fields, unsafe versions, nonboolean flags, invalid reasons, and invalid identity before fetch", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(policy));
    const api = createReportArchivePolicyApi(fetcher);
    const invalidInputs: unknown[] = [
      { ...input, starts: {} }, { ...input, version: 0 }, { ...input, version: Number.MAX_SAFE_INTEGER },
      { ...input, daily_enabled: 1 }, { ...input, monthly_enabled: "false" }, { ...input, reason: " padded " },
      { ...input, reason: "control\u0085" }, { ...input, reason: "é".repeat(251) },
      { ...input, reason: "\ud800" }, { ...input, reason: "" },
    ];
    for (const value of invalidInputs) await expect(api.update(brand, value as UpdateReportArchivePolicyInput, key, actor)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.update("bad-brand", input, key, actor)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.update(brand, input, key, "bad-actor")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("requires current brand membership and both brand permissions for a nonsuper account", () => {
    const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [] };
    expect(reportArchivePolicyPermissions({ ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_policy.write.brand"] } }, brand)).toEqual({ view: true, write: true });
    expect(reportArchivePolicyPermissions({ ...account, brand_ids: [], permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_policy.write.brand"] } }, brand)).toEqual({ view: false, write: false });
    expect(reportArchivePolicyPermissions({ ...account, permissions_by_brand: { [brand]: ["report_archive_policy.write.brand"] } }, brand)).toEqual({ view: false, write: false });
    expect(reportArchivePolicyPermissions({ ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand"] } }, brand)).toEqual({ view: true, write: false });
    expect(reportArchivePolicyPermissions({ ...account, super_admin: true, permissions_by_brand: { [brand]: ["report_archive.view.brand", "report_archive_policy.write.brand"] } }, brand)).toEqual({ view: true, write: false });
    expect(reportArchivePolicyPermissions({ ...account, super_admin: true, brand_ids: [], permissions_by_brand: {}, platform_permissions: ["report_archive.view.platform"] }, brand)).toEqual({ view: true, write: false });
  });

  it("treats mismatched acknowledgements and unknown 500 outcomes as unknown writes without automatic replay", async () => {
    for (const mismatch of [
      { ...policy, brand_id: "44444444-4444-4444-8444-444444444444" },
      { ...policy, version: 6 }, { ...policy, daily_enabled: false }, { ...policy, daily_start_period: null },
    ]) {
      await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(mismatch))).update(brand, input, key, actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    }
    const forgedInitialDisabled = { ...policy, version: 2, daily_enabled: false, monthly_enabled: false };
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(forgedInitialDisabled))).update(
      brand, { version: 1, daily_enabled: false, monthly_enabled: false, reason: "Keep archiving disabled" }, key, actor,
    )).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(failure(500)).mockResolvedValueOnce(ok(policy));
    const api = createReportArchivePolicyApi(fetcher);
    await expect(api.update(brand, input, key, actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    expect(fetcher).toHaveBeenCalledOnce();
    await expect(api.update(brand, input, key, actor)).resolves.toEqual(policy);
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(fetcher.mock.calls.map(([, init]) => [init?.body, new Headers(init?.headers).get("Idempotency-Key")])).toEqual([
      [JSON.stringify(input), key], [JSON.stringify(input), key],
    ]);
  });

  it("preserves definitive 409 conflicts and rejects non-200 successful statuses as unknown", async () => {
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(failure(409))).update(brand, input, key, actor)).rejects.toMatchObject({ status: 409, code: "ERROR_409" });
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(ok(policy, 201))).update(brand, input, key, actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).update(brand, input, key, actor)).rejects.toBeInstanceOf(AdminApiError);
    await expect(createReportArchivePolicyApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).update(brand, input, key, actor)).rejects.toMatchObject({ code: "UNKNOWN_WRITE_STATUS" });
  });
});
