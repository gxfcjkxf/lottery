import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createReconciliationApi, reconciliationPermissions, type ReconciliationJob,
} from "./reconciliation-api";

const brand = "11111111-1111-4111-8111-111111111111";
const accountId = "22222222-2222-4222-8222-222222222222";
const jobId = "33333333-3333-4333-8333-333333333333";
const auditId = "44444444-4444-4444-8444-444444444444";
const stamp = "2026-10-07T10:00:00Z";
function job(patch: Partial<ReconciliationJob> = {}): ReconciliationJob {
  return {
    id: jobId, brand_id: brand, state: "pending", version: 1,
    target_count: "2", checked_count: "0", consistent_count: "0", repairable_count: "0", corrupt_count: "0", failed_count: "0", pending_count: "2",
    created_by: accountId, reason: "scheduled audit", created_at: stamp, started_at: null, completed_at: null,
    last_error_code: null, can_retry: false, creation_audit_log_id: auditId, ...patch,
  };
}
function response(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } });
}
function admin(patch: Partial<AdminAccount> = {}): AdminAccount {
  return { id: accountId, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["wallet.view.brand", "wallet.reconcile.brand"] }, ...patch };
}

describe("reconciliation SDK", () => {
  it("applies platform wallet view independently of membership but requires member brand permissions to write", () => {
    expect(reconciliationPermissions(admin({ brand_ids: [], permissions_by_brand: {}, platform_permissions: ["wallet.view.platform"] }), brand))
      .toEqual({ view: true, run: false, retry: false });
    expect(reconciliationPermissions(admin({ super_admin: true, platform_permissions: ["wallet.view.platform"] }), brand))
      .toEqual({ view: true, run: false, retry: false });
    expect(reconciliationPermissions(admin({ permissions_by_brand: undefined, permissions: ["wallet.view.brand", "wallet.reconcile.brand"] }), brand))
      .toEqual({ view: true, run: true, retry: true });
  });

  it("sends same-origin scoped list and validates the complete page contract", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, items: [job()], total_count: "1", limit: 20, offset: 0 }));
    const page = await createReconciliationApi(fetcher).list(brand);
    expect(page.items[0].id).toBe(jobId);
    expect(fetcher).toHaveBeenCalledWith(`/api/v1/admin/reconciliations?limit=20&offset=0`, expect.objectContaining({ credentials: "same-origin" }));
    const init = fetcher.mock.calls[0][1]!;
    expect(new Headers(init.headers).get("X-Brand-ID")).toBe(brand);
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, items: [job({ pending_count: "1" })], total_count: "1", limit: 20, offset: 0 }))).list(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: accountId, items: [], total_count: "0", limit: 20, offset: 0 }))).list(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("freezes a create request on the wire and accepts only its pending v1 receipt", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(job()));
    const api = createReconciliationApi(fetcher);
    const body = { reason: "scheduled audit" };
    await expect(api.create(brand, body, "recon-create-key-001", accountId)).resolves.toEqual(job());
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/reconciliations");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(JSON.stringify(body));
    const headers = new Headers(init?.headers);
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe("recon-create-key-001");
    expect(headers.get("Content-Type")).toBe("application/json");
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job({ checked_count: "1", consistent_count: "1", pending_count: "1" }))))
      .create(brand, body, "recon-create-key-001", accountId)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
  });

  it("loads current job and outcome-filtered targets with exact cross-scope checks", async () => {
    const target = { id: auditId, brand_id: brand, job_id: jobId, account_id: accountId, member_id: "55555555-5555-4555-8555-555555555555",
      state: "pending", outcome: null, preview: null, attempt_count: 0, error_code: null, checked_at: null, audit_log_id: null };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(job()))
      .mockResolvedValueOnce(response({ brand_id: brand, job_id: jobId, items: [target], total_count: "1", limit: 20, offset: 0, outcome: "pending" }));
    const api = createReconciliationApi(fetcher);
    await expect(api.read(brand, jobId)).resolves.toEqual(job());
    const result = await api.targets(brand, jobId, "pending");
    expect(result.items[0].state).toBe("pending");
    expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/admin/reconciliations/${jobId}/targets?limit=20&offset=0&outcome=pending`);
    const bad = { ...target, brand_id: accountId };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, job_id: jobId, items: [bad], total_count: "1", limit: 20, offset: 0, outcome: null })))
      .targets(brand, jobId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("accepts zero-version historical previews with sparse actual buckets and nonnegative expected buckets", async () => {
    const member = "55555555-5555-4555-8555-555555555555";
    const expected = Object.fromEntries(["recharge", "winning", "gift", "commission"].map((source) => [source,
      Object.fromEntries(["available", "manual_frozen", "system_frozen", "withdrawal"].map((state) => [state, "0"]))]));
    const preview = { account_id: accountId, member_id: member, version: 0, ledger_version: 0, actual: {}, expected,
      repairable: true, consistent: false, issues: ["missing balance buckets"], token: "a".repeat(64) };
    const target = { id: auditId, brand_id: brand, job_id: jobId, account_id: accountId, member_id: member, state: "checked", outcome: "repairable",
      preview, attempt_count: 1, error_code: null, checked_at: stamp, audit_log_id: "66666666-6666-4666-8666-666666666666" };
    const page = { brand_id: brand, job_id: jobId, items: [target], total_count: "1", limit: 20, offset: 0, outcome: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(page))).targets(brand, jobId)).resolves.toMatchObject({ items: [target] });
    const falsePass={...target,outcome:"consistent",preview:{...preview,consistent:true,repairable:false,issues:[]}};
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({...page,items:[falsePass]}))).targets(brand,jobId)).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    const negativeExpected = { ...preview, expected: { ...expected, recharge: { ...expected.recharge, available: "-1" } } };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [{ ...target, preview: negativeExpected }] })))
      .targets(brand, jobId)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const signedActual = { ...preview, actual: { recharge: { available: "-1" } } };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [{ ...target, preview: signedActual }] })))
      .targets(brand, jobId)).resolves.toMatchObject({ items: [{ preview: signedActual }] });
    const legacyBuckets = Object.fromEntries(["recharge", "winning", "gift"].map((source) => [source,
      Object.fromEntries(["available", "manual_frozen", "system_frozen", "withdrawal"].map((state) => [state, "0"]))]));
    const legacyPreview = { ...preview, actual: legacyBuckets, expected: legacyBuckets, consistent: true, repairable: false, issues: [] };
    const legacyPage = { ...page, items: [{ ...target, outcome: "consistent", preview: legacyPreview }] };
    const read = await createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(legacyPage))).targets(brand, jobId);
    expect(read.items[0]?.preview?.actual).toHaveProperty("commission.available", "0");
    expect(read.items[0]?.preview?.expected).toHaveProperty("commission.available", "0");
    expect(Object.keys(legacyBuckets)).toEqual(["recharge", "winning", "gift"]);
  });

  it("validates running and completed job lifecycle counters and timestamps", async () => {
    const running = job({ state: "running", started_at: stamp });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(running))).read(brand, jobId)).resolves.toEqual(running);
    const runningWithoutStart = { ...running, started_at: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(runningWithoutStart))).read(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const completed = job({ state: "completed", checked_count: "2", consistent_count: "2", pending_count: "0", started_at:stamp, completed_at: stamp });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(completed))).read(brand, jobId)).resolves.toEqual(completed);
    for(const damaged of [{...completed,started_at:null},{...completed,completed_at:null},{...completed,completed_at:"2026-10-06T10:00:00Z"}]){
      await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(damaged))).read(brand,jobId)).rejects.toMatchObject({code:"INVALID_RESPONSE"});
    }
    const incomplete = { ...completed, checked_count: "1", consistent_count: "1", pending_count: "1" };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(incomplete))).read(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("accepts the immutable pending retry receipt even when a subsequent read may advance state", async () => {
    const previous = job({ state: "failed", version: 4, checked_count: "1", consistent_count: "1", failed_count: "1", pending_count: "0", started_at: stamp, completed_at: null, last_error_code: "CHECK_FAILED", can_retry: true });
    const receipt = job({ state: "pending", version: 5, started_at: stamp, checked_count: "1", consistent_count: "1", failed_count: "0", pending_count: "1" });
    const currentCompleted = job({ state: "completed", version: 6, checked_count: "2", consistent_count: "2", pending_count: "0", started_at: stamp, completed_at: stamp });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(response(receipt)).mockResolvedValueOnce(response(currentCompleted));
    const api = createReconciliationApi(fetcher);
    await expect(api.retry(brand, jobId, { version: 4, reason: "retry failed checks" }, "recon-retry-key-001", previous)).resolves.toEqual(receipt);
    expect(fetcher.mock.calls[0][0]).toBe(`/api/v1/admin/reconciliations/${jobId}/retry`);
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body))).toEqual({ version: 4, reason: "retry failed checks" });
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("Idempotency-Key")).toBe("recon-retry-key-001");
    await expect(api.read(brand, jobId)).resolves.toEqual(currentCompleted);
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(receipt)))
      .retry(brand, jobId, { version: 4, reason: "retry failed checks" }, "recon-retry-key-001"))
      .resolves.toEqual(receipt);
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(receipt))).retry(brand, jobId, { version: 4, reason: "retry failed checks" }, "recon-retry-key-001", currentCompleted))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...receipt, creation_audit_log_id: "66666666-6666-4666-8666-666666666666" })))
      .retry(brand, jobId, { version: 4, reason: "retry failed checks" }, "recon-retry-key-001", previous))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...receipt, started_at: null })))
      .retry(brand, jobId, { version: 4, reason: "retry failed checks" }, "recon-retry-key-001", previous))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
  });

  it("rejects malformed envelopes, IDs, input, and uncertain success payloads", async () => {
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job()))).read(brand, "not-an-id"))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(new Response("{}", { status: 200 }))).list(brand))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({}))).create(brand, { reason: "ok" }, "recon-create-key-001", accountId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline"))).create(brand, { reason: "ok" }, "recon-create-key-001"))
      .rejects.toBeInstanceOf(AdminApiError);
  });
});
