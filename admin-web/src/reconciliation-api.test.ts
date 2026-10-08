import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  BUSINESS_FAMILIES, createReconciliationApi, reconciliationPermissions, type BusinessPreview, type ReconciliationJob,
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
function businessPreview(memberId = "55555555-5555-4555-8555-555555555555", patch: Partial<BusinessPreview> = {}): BusinessPreview {
  return {
    account_id: accountId, member_id: memberId, account_version: 0, ledger_entry_count: "0", business_reference_count: "0", issue_count: "0",
    issues_truncated: false, consistent: true, fingerprint: "a".repeat(64), issues: [],
    coverage: BUSINESS_FAMILIES.map((family) => ({ family, ledger_entry_count: "0", business_reference_count: "0", issue_count: "0" })), ...patch,
  };
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

  it("sends explicit full scope and binds the immutable receipt scope", async () => {
    const fullJob = job({ reason: "full audit", check_scope: "wallet_and_business" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(fullJob));
    await expect(createReconciliationApi(fetcher).create(brand, { reason: "full audit", check_scope: "wallet_and_business" }, "recon-full-key-001", accountId))
      .resolves.toEqual(fullJob);
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body))).toEqual({ reason: "full audit", check_scope: "wallet_and_business" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job())))
      .create(brand, { reason: "full audit", check_scope: "wallet_and_business" }, "recon-full-key-001", accountId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(fullJob)))
      .create(brand, { reason: "wallet audit" }, "recon-full-key-001", accountId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...fullJob, check_scope: "wallet_and_other" })))
      .read(brand, jobId)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
  });

  it("accepts only complete, internally consistent full-scope business previews", async () => {
    const member = "55555555-5555-4555-8555-555555555555";
    const expected = Object.fromEntries(["recharge", "winning", "gift", "commission"].map((source) => [source,
      Object.fromEntries(["available", "manual_frozen", "system_frozen", "withdrawal"].map((state) => [state, "0"]))]));
    const wallet = { account_id: accountId, member_id: member, version: 0, ledger_version: 0, actual: {}, expected,
      repairable: true, consistent: false, issues: ["missing balance buckets"], token: "b".repeat(64) };
    const checked = { id: auditId, brand_id: brand, job_id: jobId, account_id: accountId, member_id: member, state: "checked", outcome: "repairable",
      preview: wallet, attempt_count: 1, error_code: null, checked_at: stamp, audit_log_id: "66666666-6666-4666-8666-666666666666",
      check_scope: "wallet_and_business", business_preview: businessPreview(member) };
    const page = { brand_id: brand, job_id: jobId, items: [checked], total_count: "1", limit: 20, offset: 0, outcome: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(page))).targets(brand, jobId, null, 20, 0, "wallet_and_business"))
      .resolves.toMatchObject({ items: [{ outcome: "repairable" }] });
    const malformed = [
      { ...checked, business_preview: null },
      { ...checked, outcome: "consistent" },
      { ...checked, business_preview: { ...checked.business_preview, account_version: 1 } },
      { ...checked, business_preview: { ...checked.business_preview, coverage: checked.business_preview.coverage.slice(1) } },
      { ...checked, business_preview: { ...checked.business_preview, ledger_entry_count: "1" } },
      { ...checked, business_preview: { ...checked.business_preview, issue_count: "1" } },
      { ...checked, business_preview: { ...checked.business_preview, surprise: true } },
      { ...checked, business_preview: { ...checked.business_preview, issues: Array.from({ length: 101 }, () => ({})) } },
      { ...checked, business_preview: { ...checked.business_preview, coverage: checked.business_preview.coverage.map((row, index) => index === 1 ? { ...row, family: "bet" } : row) } },
      { ...checked, business_preview: { ...checked.business_preview, coverage: checked.business_preview.coverage.map((row, index) => index === 0 ? { ...row, ledger_entry_count: "1" } : row) } },
      { ...checked, business_preview: { ...checked.business_preview, issue_count: "1" } },
      { ...checked, business_preview: { ...checked.business_preview, issues_truncated: true } },
    ];
    for (const item of malformed) {
      await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [item] }))).targets(brand, jobId))
        .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const businessIssue = { code: "MISSING_BUSINESS_RECORD", entry_type: "recharge", ledger_entry_id: auditId, resource_type: "recharge", resource_id: null } as const;
    const inconsistent = businessPreview(member, { issue_count: "1", consistent: false, issues: [businessIssue],
      coverage: BUSINESS_FAMILIES.map((family) => ({ family, ledger_entry_count: "0", business_reference_count: "0", issue_count: family === "recharge" ? "1" : "0" })) });
    const corrupt = { ...checked, outcome: "corrupt", business_preview: inconsistent };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [corrupt] }))).targets(brand, jobId))
      .resolves.toMatchObject({ items: [{ outcome: "corrupt" }] });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [{ ...corrupt, outcome: "repairable" }] }))).targets(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const pendingFull = { ...checked, state: "pending", outcome: null, preview: null, attempt_count: 0, checked_at: null, audit_log_id: null, business_preview: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [pendingFull] }))).targets(brand, jobId))
      .resolves.toMatchObject({ items: [{ state: "pending" }] });
    const failedFull = { ...checked, state: "failed", outcome: null, preview: null, attempt_count: 1, error_code: "CHECK_FAILED", checked_at: null, audit_log_id: null, business_preview: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [failedFull] }))).targets(brand, jobId))
      .resolves.toMatchObject({ items: [{ state: "failed", business_preview: null }] });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [{ ...failedFull, business_preview: undefined }] }))).targets(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("accepts exact ASCII ledger tags and validates bigint-safe business aggregates", async () => {
    const member = "55555555-5555-4555-8555-555555555555";
    const tag = "AbC_9.:--" + "x".repeat(191);
    const count = "9007199254740992";
    const coverage = BUSINESS_FAMILIES.map((family, index) => ({ family, ledger_entry_count: "0", business_reference_count: count, issue_count: index === 0 ? "1" : "0" }));
    const references = (BigInt(count) * BigInt(BUSINESS_FAMILIES.length)).toString();
    const expected = Object.fromEntries(["recharge", "winning", "gift", "commission"].map((source) => [source,
      Object.fromEntries(["available", "manual_frozen", "system_frozen", "withdrawal"].map((state) => [state, "0"]))]));
    const wallet = { account_id: accountId, member_id: member, version: 0, ledger_version: 0, actual: {}, expected,
      repairable: false, consistent: false, issues: ["legacy wallet observation"], token: "b".repeat(64) };
    const preview = businessPreview(member, {
      business_reference_count: references, issue_count: "1", consistent: false, coverage,
      issues: [{ code: "UNSUPPORTED_LEDGER_TYPE", entry_type: tag, ledger_entry_id: auditId, resource_type: "ledger", resource_id: auditId }],
    });
    const checked = { id: auditId, brand_id: brand, job_id: jobId, account_id: accountId, member_id: member, state: "checked", outcome: "corrupt",
      preview: wallet, attempt_count: 1, error_code: null, checked_at: stamp, audit_log_id: "66666666-6666-4666-8666-666666666666",
      check_scope: "wallet_and_business", business_preview: preview };
    const page = { brand_id: brand, job_id: jobId, items: [checked], total_count: "1", limit: 20, offset: 0, outcome: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(page))).targets(brand, jobId))
      .resolves.toMatchObject({ items: [{ business_preview: { business_reference_count: "108086391056891904", issues: [{ entry_type: tag }] } }] });

    const sampledIssue = { code: "MISSING_BUSINESS_RECORD", entry_type: "recharge", ledger_entry_id: auditId, resource_type: "recharge", resource_id: null };
    const truncatedPreview = { ...preview, issue_count: "101", issues_truncated: true,
      coverage: coverage.map((row, index) => ({ ...row, issue_count: index === 0 ? "101" : "0" })),
      issues: Array.from({ length: 100 }, () => sampledIssue) };
    const truncated = await createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [{ ...checked, business_preview: truncatedPreview }] })))
      .targets(brand, jobId);
    expect(truncated.items[0]?.business_preview).toMatchObject({ issue_count: "101", issues_truncated: true });
    expect(truncated.items[0]?.business_preview?.issues).toHaveLength(100);

    for (const invalidTag of ["x".repeat(201), "ledger-é"]) {
      const invalid = { ...checked, business_preview: { ...preview, issues: [{ ...preview.issues[0], entry_type: invalidTag }] } };
      await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [invalid] }))).targets(brand, jobId))
        .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const tooManyIssues = { ...checked, business_preview: { ...preview, issue_count: "101", issues_truncated: true,
      coverage: coverage.map((row, index) => ({ ...row, issue_count: index === 0 ? "101" : "0" })),
      issues: Array.from({ length: 101 }, () => preview.issues[0]) } };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...page, items: [tooManyIssues] }))).targets(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("retains legacy wallet receipts while rejecting mixed-scope and unsafe wire shapes", async () => {
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job({ reason: "legacy" }))))
      .create(brand, { reason: "legacy" }, "recon-legacy-key-001", accountId)).resolves.toMatchObject({ id: jobId });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job({ reason: "legacy" }))))
      .create(brand, { reason: "legacy", check_scope: "wallet", unknown: true } as never, "recon-legacy-key-001", accountId))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(job({ reason: "legacy" }))))
      .create(brand, { reason: "legacy", check_scope: "business" as never }, "recon-legacy-key-001", accountId))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    const mismatchedTarget = { id: auditId, brand_id: brand, job_id: jobId, account_id: accountId, member_id: "55555555-5555-4555-8555-555555555555",
      state: "pending", outcome: null, preview: null, attempt_count: 0, error_code: null, checked_at: null, audit_log_id: null,
      check_scope: "wallet_and_business", business_preview: null };
    const targetPage = { brand_id: brand, job_id: jobId, items: [mismatchedTarget], total_count: "1", limit: 20, offset: 0, outcome: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(targetPage))).targets(brand, jobId, null, 20, 0, "wallet"))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badScopeTarget = { ...mismatchedTarget, check_scope: "wallet_and_business_extra", business_preview: null };
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...targetPage, items: [badScopeTarget] }))).targets(brand, jobId))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("reads legacy inline-tab reasons without changing the stricter UI write policy", async () => {
    const legacy = job({ reason: "inline\tseparator" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(legacy))).read(brand, jobId))
      .resolves.toEqual(legacy);
  });

  it("binds retry receipts to the original immutable full scope", async () => {
    const previous = job({ check_scope: "wallet_and_business", state: "failed", version: 4, checked_count: "1", consistent_count: "1", failed_count: "1", pending_count: "0", started_at: stamp, last_error_code: "CHECK_FAILED", can_retry: true });
    const receipt = job({ check_scope: "wallet_and_business", state: "pending", version: 5, started_at: stamp, checked_count: "1", consistent_count: "1", pending_count: "1" });
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response(receipt)))
      .retry(brand, jobId, { version: 4, reason: "retry full check" }, "recon-retry-scope-001", previous)).resolves.toEqual(receipt);
    await expect(createReconciliationApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...receipt, check_scope: "wallet" })))
      .retry(brand, jobId, { version: 4, reason: "retry full check" }, "recon-retry-scope-001", previous))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 0 });
  });

  it("keeps exact UTF-8 reason limits and safe retry version bounds", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(job({ reason: "x".repeat(500) })));
    await expect(createReconciliationApi(fetcher).create(brand, { reason: "x".repeat(500) }, "recon-limit-key-001", accountId)).resolves.toBeDefined();
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body)).reason).toHaveLength(500);
    await expect(createReconciliationApi(fetcher).create(brand, { reason: "x".repeat(501) }, "recon-limit-key-002", accountId))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReconciliationApi(fetcher).create(brand, { reason: "界".repeat(167) }, "recon-limit-key-003", accountId))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(createReconciliationApi(fetcher).retry(brand, jobId, { version: Number.MAX_SAFE_INTEGER, reason: "retry" }, "recon-limit-key-004"))
      .rejects.toMatchObject({ code: "INVALID_INPUT" });
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
