import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { commissionPaymentPermissions, createCommissionPaymentsApi as createRawCommissionPaymentsApi, type CommissionPayment } from "./commissionPayments-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const createCommissionPaymentsApi = (fetcher: typeof fetch) => createRawCommissionPaymentsApi(fetcher, actor);
const id = "44444444-4444-4444-8444-444444444444";
const cycle = "55555555-5555-4555-8555-555555555555";
const run = "66666666-6666-4666-8666-666666666666";
const audit = "77777777-7777-4777-8777-777777777777";
const now = "2026-10-07T00:00:00Z";
const base: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions_by_brand: { [brand]: ["commission.view.brand", "commission_payment_policy.write.brand", "commission_payment.approve.brand", "commission_payment.retry.brand"] }, permissions: [] };

function payment(overrides: Partial<CommissionPayment> = {}): CommissionPayment {
  return { id, brand_id: brand, cycle_id: cycle, run_id: run, state: "failed", payout_mode: "automatic", version: 2, evidence_epoch: "900719925474099312345", total_points: "900719925474099312345", paid_points: "900719925474099300000", target_count: "900719925474099312345", paid_count: "900719925474099300000", creation_audit_log_id: audit, last_error_code: "POST_FAILED", created_at: now, updated_at: now, ...overrides };
}
function ok(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data }), { status }); }

describe("commission payments API", () => {
  it("keeps payment permissions brand scoped and denies super-admin writes", () => {
    expect(commissionPaymentPermissions(base, brand)).toEqual({ view: true, policyWrite: true, approve: true, retry: true });
    expect(commissionPaymentPermissions(base, otherBrand).view).toBe(false);
    expect(commissionPaymentPermissions({ ...base, super_admin: true, platform_permissions: ["commission.view.platform"], permissions_by_brand: { [brand]: base.permissions_by_brand![brand] } }, brand)).toEqual({ view: true, policyWrite: false, approve: false, retry: false });
  });

  it("sends exact versioned bodies, brand and idempotency headers for all six routes", async () => {
    const fetcher = vi.fn<typeof fetch>(async (input, init) => {
      const url = String(input), method = init?.method ?? "GET";
      if (url.endsWith("commission-payment-policy") && method === "GET") return ok({ brand_id: brand, version: 1, enabled: false, audit_log_id: "", updated_at: now });
      if (url.endsWith("commission-payment-policy")) return ok({ brand_id: brand, version: 2, enabled: true, audit_log_id: audit, updated_at: now });
      if (url.endsWith("/approve")) return ok(payment({ version: 3, state: "paying", payout_mode: "manual", last_error_code: null }));
      if (url.endsWith("/retry")) return ok(payment({ version: 3, state: "paying", last_error_code: null }));
      if (url.endsWith("/commission-payments?limit=20&offset=0")) return ok({ brand_id: brand, items: [payment()], total_count: "1", limit: 20, offset: 0 });
      return ok(payment());
    });
    const api = createCommissionPaymentsApi(fetcher);
    await expect(api.getPolicy(brand)).resolves.toMatchObject({ enabled: false });
    await expect(api.list(brand)).resolves.toMatchObject({ items: [{ id }], total_count: "1" });
    await expect(api.read(brand, id)).resolves.toMatchObject({ id });
    await api.updatePolicy(brand, Object.freeze({ version: 1, enabled: true, reason: "Enable reviewed policy" }), "policy-key-0001");
    await api.approve(brand, id, Object.freeze({ version: 2, reason: "Approved after review" }), "approve-key-01");
    await api.retry(brand, id, Object.freeze({ version: 2, reason: "Resume unfinished targets" }), "retry-key-0001");
    for (const [input, init] of fetcher.mock.calls) expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    const writes = fetcher.mock.calls.filter(([, init]) => init?.method === "PUT" || init?.method === "POST");
    for (const [, init] of writes) expect(new Headers(init?.headers).get("X-Commission-Payment-Actor-ID")).toBe(actor);
    expect(writes.map(([, init]) => [init?.method, new Headers(init?.headers).get("Idempotency-Key"), init?.body])).toEqual([
      ["PUT", "policy-key-0001", '{"version":1,"enabled":true,"reason":"Enable reviewed policy"}'],
      ["POST", "approve-key-01", '{"version":2,"reason":"Approved after review"}'],
      ["POST", "retry-key-0001", '{"version":2,"reason":"Resume unfinished targets"}'],
    ]);
    expect(writes.every(([, init]) => init?.credentials === "same-origin")).toBe(true);
  });

  it("validates canonical decimal strings with BigInt precision and allows paired Unicode reasons", async () => {
    const fetcher = vi.fn<typeof fetch>(async (_input, init) => ok(init?.method === "POST" ? payment({version: 3, state: "paying", last_error_code: null}) : payment()));
    const api = createCommissionPaymentsApi(fetcher);
    await expect(api.read(brand, id)).resolves.toMatchObject({ paid_count: "900719925474099300000" });
    await expect(api.retry(brand, id, { version: 2, reason: "Reviewed ✅" }, "emoji-key-0001")).resolves.toMatchObject({ version: 3 });
    const bad = [
      payment({ paid_count: "090" }), payment({ paid_count: "900719925474099312346" }),
      payment({ paid_points: "900719925474099312346" }), payment({ updated_at: "2026-02-30T00:00:00Z" }),
      payment({ updated_at: "2026-10-07T00:00:00+14:01" }), payment({ extra: "closed shape" } as Partial<CommissionPayment>),
    ];
    for (const value of bad) await expect(createCommissionPaymentsApi(async () => ok(value)).read(brand, id)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(api.retry(brand, id, { version: 2, reason: "\ud800" }, "broken-key-0001")).rejects.toBeInstanceOf(AdminApiError);
  });

  it("accepts UTC Z timestamps and valid offsets, and requires an exact next-version write receipt", async () => {
    const api = createCommissionPaymentsApi(async () => ok(payment({ created_at: "2026-10-07T08:00:00+08:00", updated_at: "2026-10-07T00:00:00Z" })));
    await expect(api.read(brand, id)).resolves.toMatchObject({ id });
    const staleReceipt = createCommissionPaymentsApi(async () => ok(payment({ version: 5 })));
    await expect(staleReceipt.retry(brand, id, { version: 1, reason: "Recover original receipt" }, "receipt-key-0001"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects policy receipts that acknowledge a different enabled value or omit a new audit UUID", async () => {
    const body = { version: 1, enabled: true, reason: "Enable reviewed payout gate" };
    for (const receipt of [
      { brand_id: brand, version: 2, enabled: false, audit_log_id: audit, updated_at: now },
      { brand_id: brand, version: 2, enabled: true, audit_log_id: "", updated_at: now },
    ]) await expect(createCommissionPaymentsApi(async () => ok(receipt)).updatePolicy(brand, body, "policy-key-0001"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects action receipts that do not acknowledge a paying job at the original next version", async () => {
    const body = { version: 2, reason: "Reviewed retry" };
    const receipts = [
      payment({ version: 3, state: "failed" }),
      payment({ version: 3, state: "paying", last_error_code: "POST_FAILED" }),
      payment({ version: 4, state: "paying", last_error_code: null }),
    ];
    for (const receipt of receipts) await expect(createCommissionPaymentsApi(async () => ok(receipt)).retry(brand, id, body, "retry-key-0001"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCommissionPaymentsApi(async () => ok(payment({ version: 3, state: "paying", last_error_code: null }))).retry(brand, id, body, "retry-key-0001"))
      .resolves.toMatchObject({ version: 3, state: "paying" });
    await expect(createCommissionPaymentsApi(async () => ok(payment({ version: 3, state: "paying", last_error_code: null }))).approve(brand, id, body, "approve-key-001"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects array-coerced enum values and inconsistent paid, failed, blocked, and awaiting states", async () => {
    const malformed = [
      payment({ state: ["paid"] as unknown as CommissionPayment["state"] }),
      payment({ payout_mode: ["manual"] as unknown as CommissionPayment["payout_mode"] }),
      payment({ state: "paid", last_error_code: null }),
      payment({ state: "failed", last_error_code: null }),
      payment({ state: "blocked", last_error_code: null }),
      payment({ state: "paying", last_error_code: "unexpected" }),
      payment({ state: "awaiting_approval", payout_mode: "manual", paid_count: "1" }),
      payment({ state: "awaiting_approval", payout_mode: "automatic", last_error_code: null, paid_count: "0", paid_points: "0" }),
      payment({ state: "awaiting_approval", payout_mode: "none", last_error_code: null, paid_count: "0", paid_points: "0" }),
    ];
    for (const record of malformed) await expect(createCommissionPaymentsApi(async () => ok(record)).read(brand, id))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCommissionPaymentsApi(async () => ok(payment({ state: "paid", paid_count: payment().target_count, paid_points: payment().total_points, last_error_code: null }))).read(brand, id))
      .resolves.toMatchObject({ state: "paid" });
  });

  it("accepts mixed awaiting-approval snapshots and receipts without changing payout mode", async () => {
    const awaiting = payment({ state: "awaiting_approval", payout_mode: "mixed", paid_count: "0", paid_points: "0", last_error_code: null });
    await expect(createCommissionPaymentsApi(async () => ok(awaiting)).read(brand, id)).resolves.toMatchObject({ state: "awaiting_approval", payout_mode: "mixed" });
    await expect(createCommissionPaymentsApi(async () => ok({ ...awaiting, state: "paying", version: 3, last_error_code: null })).approve(brand, id, { version: 2, reason: "Review whole mixed cycle" }, "approve-key-mixed1"))
      .resolves.toMatchObject({ state: "paying", payout_mode: "mixed", version: 3 });
    for (const mode of ["automatic", "none"] as const) {
      await expect(createCommissionPaymentsApi(async () => ok(payment({ state: "paying", payout_mode: mode, version: 3, last_error_code: null }))).approve(brand, id, { version: 2, reason: "Reject incorrect approval receipt" }, "approve-key-invalid1"))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("accepts a failed mixed retry receipt while preserving the historical payout mode", async () => {
    const receipt = payment({ state: "paying", payout_mode: "mixed", version: 8, last_error_code: null });
    await expect(createCommissionPaymentsApi(async () => ok(receipt)).retry(brand, id, { version: 7, reason: "Resume approved mixed cycle" }, "retry-key-mixed01"))
      .resolves.toMatchObject({ id, state: "paying", payout_mode: "mixed", version: 8, last_error_code: null });
  });

  it("validates page bounds, unique ids, and exact totals while allowing an empty out-of-range page", async () => {
    const item = payment();
    const malformedPages = [
      { brand_id: brand, items: [item, item], total_count: "2", limit: 20, offset: 0 },
      { brand_id: brand, items: [item], total_count: "0", limit: 20, offset: 0 },
      { brand_id: brand, items: Array.from({ length: 21 }, (_, index) => payment({ id: `${String(index).padStart(8, "0")}-4444-4444-8444-444444444444` })), total_count: "21", limit: 20, offset: 0 },
    ];
    for (const page of malformedPages) await expect(createCommissionPaymentsApi(async () => ok(page)).list(brand))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCommissionPaymentsApi(async () => ok({ brand_id: brand, items: [], total_count: "0", limit: 20, offset: 40 })).list(brand, 20, 40))
      .resolves.toMatchObject({ items: [], offset: 40 });
  });
});
