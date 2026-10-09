import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createSettlementJobApi,
  settlementJobPermissions,
  type SettlementJob,
  type SettlementPolicy,
} from "./settlement-job-api";

const brand = "00000000-0000-4000-8000-000000000001";
const game = "00000000-0000-4000-8000-000000000002";
const period = "00000000-0000-4000-8000-000000000003";
const draw = "00000000-0000-4000-8000-000000000004";
const jobId = "00000000-0000-4000-8000-000000000005";
const admin = "00000000-0000-4000-8000-000000000006";
const member = "00000000-0000-4000-8000-000000000007";
const order = "00000000-0000-4000-8000-000000000008";
const calculation = "00000000-0000-4000-8000-000000000009";
const payout = "00000000-0000-4000-8000-000000000010";
const audit = "00000000-0000-4000-8000-000000000011";
const at = "2026-10-06T04:00:00Z";

function envelope(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: status < 400, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function policy(overrides: Partial<SettlementPolicy> = {}): SettlementPolicy {
  return { brand_id: brand, version: 8, mode: "manual", updated_at: at, ...overrides };
}

function job(overrides: Partial<SettlementJob> = {}): SettlementJob {
  return {
    generation:1,previous_job_id:null,correction_id:null,current:true,
    id: jobId,
    brand_id: brand,
    game_id: game,
    period_id: period,
    draw_result_id: draw,
    period_version: 9,
    policy_version: 8,
    mode: "manual",
    state: "awaiting_approval",
    version: 4,
    target_count: 2,
    created_by: admin,
    approved_by: null,
    reason: "batch settlement",
    created_at: at,
    completed_at: null,
    last_error_code: null,
    pending_count: 0,
    ready_count: 2,
    paid_count: 0,
    excluded_count: 0,
    failed_count: 0,
    prize_points: "900719925474099300000000000000000000",
    paid_points: "0",
    can_retry: false,
    ...overrides,
  };
}

function account(overrides: Partial<AdminAccount> = {}): AdminAccount {
  return { id: admin, super_admin: false, brand_ids: [brand], permissions: [], ...overrides };
}

describe("settlement job permissions", () => {
  it("requires mapped grants and denies platform and super-admin access", () => {
    const permissions = [
      "settlement.view.brand", "settlement.run.brand", "settlement.approve.brand",
      "settlement.retry.brand", "settlement_policy.view.brand", "settlement_policy.write.brand",
    ];
    expect(settlementJobPermissions(account({ permissions_by_brand: { [brand]: permissions } }), brand)).toEqual({
      view: true, run: true, approve: true, retry: true, policyView: true, policyWrite: true,
    });
    expect(settlementJobPermissions(account({ permissions_by_brand: { [brand]: permissions }, super_admin: true }), brand)).toEqual({ view: false, run:false,approve:false,retry:false,policyView: false,policyWrite: false });
    expect(settlementJobPermissions(account({
      super_admin: true,
      platform_permissions: ["settlement.view.platform", "settlement_policy.view.platform"],
    }), brand)).toMatchObject({ view: false, policyView: false, run: false, approve: false, retry: false, policyWrite: false });
    expect(settlementJobPermissions(account({ permissions: ["settlement.view.platform"] }), brand).view).toBe(false);
    expect(settlementJobPermissions(account({ brand_ids: [] , permissions_by_brand: { [brand]: permissions } }), brand)).toEqual({
      view: false, run: false, approve: false, retry: false, policyView: false, policyWrite: false,
    });
    expect(settlementJobPermissions(account({ id: "invalid", permissions_by_brand: { [brand]: permissions } }), brand).run).toBe(false);
    expect(settlementJobPermissions(account({ permissions_by_brand: { [brand]: ["settlement.run.brand"] }, permissions: ["settlement.approve.brand"] }), brand))
      .toMatchObject({ run: true, approve: false });
  });
});

describe("settlement job API", () => {
  it("fetches policy and context with included cookies and validates exact eligibility", async () => {
    const context = {
      brand_id: brand,
      game_id: game,
      period_id: period,
      period_version: 9,
      period_status: "drawn",
      draw_result_id: draw,
      policy_version: 8,
      mode: "manual",
      can_start: true,
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(envelope(policy()))
      .mockResolvedValueOnce(envelope(context));
    const api = createSettlementJobApi({ fetch: fetcher });
    await expect(api.policy(brand)).resolves.toEqual(policy());
    await expect(api.context(brand, period)).resolves.toEqual(context);
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/admin/settlement-policy",
      `/api/v1/admin/periods/${period}/settlement-context`,
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("include");
      expect((init?.headers as Headers).get("X-Brand-ID")).toBe(brand);
    }
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({
      ...context, mode: null, can_start: true,
    })) }).context(brand, period)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({
      ...context, period_status: "open", draw_result_id: null, can_start: false,
    })) }).context(brand, period)).resolves.toMatchObject({ can_start: false });
  });

  it("sends policy writes verbatim and requires the exact next version, mode and audit receipt", async () => {
    const body = { version: 8, mode: "automatic" as const, reason: "enable scheduled settlement" };
    const receipt = { ...policy({ version: 9, mode: "automatic" }), audit_log_id: audit };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(receipt));
    await expect(createSettlementJobApi({ fetch: fetcher }).savePolicy(brand, body, "policy-key"))
      .resolves.toEqual(receipt);
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/settlement-policy");
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("include");
    const headers = init?.headers as Headers;
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe("policy-key");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    for (const bad of [
      { ...receipt, version: 10 },
      { ...receipt, mode: "manual" },
      { ...receipt, audit_log_id: "bad" },
      { ...receipt, brand_id: "00000000-0000-4000-8000-000000000099" },
    ]) {
      await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(bad)) })
        .savePolicy(brand, body, "key")).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("posts settlement with caller-owned idempotency and permits only requested source scope", async () => {
    const body = { version: 9, policy_version: 8, draw_result_id: draw, reason: "close period" };
    const initial = {state:"processing" as const,version:1,period_version:10,pending_count:2,ready_count:0};
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(job(initial), 201));
    await createSettlementJobApi({ fetch: fetcher }).start(brand, period, body, "start-stable-key", admin);
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/periods/${period}/settle`);
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("include");
    expect((init?.headers as Headers).get("X-Brand-ID")).toBe(brand);
    expect((init?.headers as Headers).get("Idempotency-Key")).toBe("start-stable-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    for (const bad of [
      job({ ...initial,period_id: "00000000-0000-4000-8000-000000000099" }),
      job({ ...initial,draw_result_id: "00000000-0000-4000-8000-000000000099" }),
      job({ ...initial,policy_version: 7 }),
      job({ ...initial,created_by: "00000000-0000-4000-8000-000000000099" }),
      job({ ...initial,period_version:9 }),
    ]) {
      await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(bad, 201)) })
        .start(brand, period, body, "key", admin)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("requires the exact action receipt then uses separate GETs for worker progress", async () => {
    const actionBody = { version: 4, reason: "operator approval" };
    const progressed = job({ state: "paying", version: 5, approved_by: admin, ready_count: 1, paid_count: 1, pending_count: 0 });
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => envelope(progressed));
    const api = createSettlementJobApi({ fetch: fetcher });
    await api.approve(brand, jobId, actionBody, "approve-key");
    await api.retry(brand, jobId, actionBody, "retry-key");
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
      `/api/v1/admin/settlement-jobs/${jobId}/approve`,
      `/api/v1/admin/settlement-jobs/${jobId}/retry`,
    ]);
    for (const [index, [, init]] of fetcher.mock.calls.entries()) {
      expect(init?.method).toBe("POST");
      expect((init?.headers as Headers).get("Idempotency-Key")).toBe(index === 0 ? "approve-key" : "retry-key");
      expect(JSON.parse(String(init?.body))).toEqual(actionBody);
    }
  });

  it("supports nullable period jobs and checks brand, period and job identity", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(envelope({ settlement: null }))
      .mockResolvedValueOnce(envelope({ settlement: job() }))
      .mockResolvedValueOnce(envelope(job()));
    const api = createSettlementJobApi({ fetch: fetcher });
    await expect(api.periodJob(brand, period)).resolves.toEqual({ settlement: null });
    await expect(api.periodJob(brand, period)).resolves.toEqual({ settlement: job() });
    await expect(api.job(brand, jobId)).resolves.toEqual(job());
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({ settlement: job({ brand_id: "00000000-0000-4000-8000-000000000099" }) })) }).periodJob(brand, period))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({ settlement: job({ period_id: "00000000-0000-4000-8000-000000000099" }) })) }).periodJob(brand, period))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const wrongIdFetcher = vi.fn<typeof fetch>().mockResolvedValue(
      envelope(job({ id: "00000000-0000-4000-8000-000000000099" })),
    );
    await expect(createSettlementJobApi({ fetch: wrongIdFetcher }).job(brand, jobId))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("pages target records and validates state, counts, dates, amounts and unsafe versions", async () => {
    const target = {
      order_id: order, member_id: member, state: "paid", version: 2,
      calculation_id: calculation, order_version: 5, order_status: "placed",
      won: true, prize_points: "9223372036854775807", payout_entry_id: payout, error_code: null,
    };
    const page = { brand_id: brand, job_id: jobId, items: [target], limit: 20, offset: 0, has_more: false };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(page));
    await expect(createSettlementJobApi({ fetch: fetcher }).targets(brand, jobId)).resolves.toEqual(page);
    expect(fetcher.mock.calls[0][0]).toBe(`/api/v1/admin/settlement-jobs/${jobId}/targets?limit=20&offset=0`);
    for (const bad of [
      { ...page, job_id: period },
      { ...page, items: [{ ...target, state: "mystery" }] },
      { ...page, items: [{ ...target, prize_points: "9223372036854775808" }] },
      { ...page, items: [{ ...target, version: Number.MAX_SAFE_INTEGER + 1 }] },
    ]) {
      await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(bad)) })
        .targets(brand, jobId)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createSettlementJobApi({ fetch: fetcher }).targets(brand, jobId, 101)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createSettlementJobApi({ fetch: fetcher }).targets(brand, jobId, 20, -1)).rejects.toMatchObject({ code: "INVALID_INPUT" });
  });

  it("preserves arbitrary precision totals and rejects malformed job invariants", async () => {
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(job())) }).job(brand, jobId))
      .resolves.toMatchObject({ prize_points: "900719925474099300000000000000000000" });
    const badJobs = [
      job({ pending_count: 2 }),
      job({ prize_points: "01" }),
      job({ paid_points: "-1" }),
      job({ created_at: "2026-02-30T12:00:00Z" }),
      job({ state: "unknown" as SettlementJob["state"] }),
      job({ version: Number.MAX_SAFE_INTEGER + 1 }),
    ];
    for (const bad of badJobs) {
      await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(bad)) }).job(brand, jobId))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("keeps malformed successful receipts and network failures distinct from explicit server errors", async () => {
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({}, 201)) }).start(
      brand, period, { version: 9, policy_version: 8, draw_result_id: draw, reason: "run" }, "key",
    )).rejects.toBeInstanceOf(AdminApiError);
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockRejectedValue(new TypeError("offline")) }).job(brand, jobId))
      .rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    await expect(createSettlementJobApi({ fetch: vi.fn<typeof fetch>().mockResolvedValue(new Response("{}", { status: 503 })) }).retry(
      brand, jobId, { version: 4, reason: "recover" }, "retry",
    )).rejects.toMatchObject({ status: 503 });
  });

  it("rejects invalid local resource scopes before making a request", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(policy()));
    const api = createSettlementJobApi({ fetch: fetcher });
    await expect(api.policy("not-a-uuid")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.context(brand, "not-a-uuid")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.start(brand, period, { version: 0, policy_version: 8, draw_result_id: draw, reason: "run" }, "key"))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });
});
