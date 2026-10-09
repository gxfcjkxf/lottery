import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  commissionCorrectionPermissions,
  createCommissionCorrectionsApi,
  type CorrectionExecution,
  type CorrectionExecutionTarget,
  type CorrectionPlan,
  type CorrectionPlanTarget,
} from "./commission-corrections-api";

const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const actor = "33333333-3333-4333-8333-333333333333";
const planID = "44444444-4444-4444-8444-444444444444";
const executionID = "55555555-5555-4555-8555-555555555555";
const cycleID = "66666666-6666-4666-8666-666666666666";
const paymentID = "77777777-7777-4777-8777-777777777777";
const runID = "88888888-8888-4888-8888-888888888888";
const auditID = "99999999-9999-4999-8999-999999999999";
const targetID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const agentID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const memberID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
const now = "2026-10-08T00:00:00Z";
const body = { version: 2, reason: "Reviewed exact correction" };
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["commission.view.brand"] } };

function plan(overrides: Partial<CorrectionPlan> = {}): CorrectionPlan {
  return { id: planID, brand_id: brand, cycle_id: cycleID, payment_id: paymentID, run_id: runID, state: "ready", payout_mode: "mixed", version: 2, evidence_epoch: "9007199254740993", before_points: "100", calculated_points: "120", credit_points: "20", debit_points: "0", net_points: "20", target_count: "1", planned_count: "1", creation_audit_log_id: auditID, last_audit_log_id: auditID, last_error_code: null, created_at: now, updated_at: now, ...overrides };
}
function planTarget(overrides: Partial<CorrectionPlanTarget> = {}): CorrectionPlanTarget {
  return { id: targetID, brand_id: brand, plan_id: planID, agent_id: agentID, member_id: memberID, original_target_id: auditID, earning_id: null, adjustment_version: 1, previous_correction_target_id: null, financial_version: null, points_before: "100", points_after: "120", delta_points: "20", creation_audit_log_id: auditID, created_at: now, ...overrides };
}
function execution(overrides: Partial<CorrectionExecution> = {}): CorrectionExecution {
  return { id: executionID, brand_id: brand, cycle_id: cycleID, plan_id: planID, run_id: runID, payout_mode: "mixed", state: "awaiting_approval", version: 2, plan_version: 2, evidence_epoch: "9007199254740993", credit_points: "9223372036854775808", debit_points: "7", net_points: "9223372036854775801", applied_credit_points: "0", applied_debit_points: "0", target_count: "1", applied_count: "0", paused_plan_target_id: null, last_error_code: null, creation_audit_log_id: auditID, last_audit_log_id: auditID, approved_by: null, approval_actor_type: null, approval_audit_log_id: null, cycle_hold_active: false, created_at: now, updated_at: now, ...overrides };
}
function executionTarget(overrides: Partial<CorrectionExecutionTarget> = {}): CorrectionExecutionTarget {
  return { id: targetID, brand_id: brand, execution_id: executionID, plan_target_id: paymentID, agent_id: agentID, member_id: memberID, points_before: "9223372036854775807", points_after: "9223372036854775806", delta_points: "-1", state: "pending", ledger_entry_id: null, audit_log_id: null, financial_version: null, created_at: now, applied_at: null, ...overrides };
}
function ok(data: unknown) { return new Response(JSON.stringify({ success: true, data }), { status: 200 }); }
const withActor = (fetcher: typeof fetch) => createCommissionCorrectionsApi(fetcher, actor);

describe("commission corrections API", () => {
  it("keeps each view and write grant independent, brand scoped, and unavailable to super-admin writes", () => {
    const expected = { view: true, policyWrite: false, planRetry: false, approve: false, continue: false, retry: false };
    expect(commissionCorrectionPermissions(account, brand)).toEqual(expected);
    const grants = ["commission.view.brand", "commission_correction_policy.write.brand", "commission_correction.retry.brand", "commission_correction.approve.brand", "commission_correction.continue.brand", "commission_correction.execute_retry.brand"];
    const full = { ...account, permissions_by_brand: { [brand]: grants } };
    expect(commissionCorrectionPermissions(full, brand)).toEqual({ view: true, policyWrite: true, planRetry: true, approve: true, continue: true, retry: true });
    expect(commissionCorrectionPermissions({ ...full, permissions_by_brand: { [brand]: grants.slice(0, 2) } }, brand).planRetry).toBe(false);
    expect(commissionCorrectionPermissions({ ...full, permissions_by_brand: { [brand]: ["commission.view.brand", "commission_correction.retry.brand"] } }, brand).retry).toBe(false);
    expect(commissionCorrectionPermissions({ ...full, permissions_by_brand: { [brand]: grants }, brand_ids: [] }, brand)).toMatchObject({ view: false, policyWrite: false, planRetry: false, approve: false, continue: false, retry: false });
    expect(commissionCorrectionPermissions({ ...full, super_admin: true, platform_permissions: ["commission.view.platform"] }, brand)).toMatchObject({ view: false, policyWrite: false, planRetry: false, approve: false, continue: false, retry: false });
    expect(commissionCorrectionPermissions({ ...full, permissions_by_brand: { [otherBrand]: grants }, platform_permissions: ["commission.view.platform"] }, brand)).toMatchObject({ view: false, policyWrite: false, planRetry: false, approve: false, continue: false, retry: false });
    expect(commissionCorrectionPermissions({ ...full, permissions: grants, permissions_by_brand: {} }, brand).view).toBe(false);
  });

  it("calls all twelve routes with same-origin credentials, captured actor, and exact write receipts", async () => {
    const policy = { brand_id: brand, version: 1, enabled: false, audit_log_id: "", updated_at: now };
    const fetcher = vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input), method = init?.method ?? "GET";
      if (path.endsWith("commission-correction-policy") && method === "GET") return ok(policy);
      if (path.endsWith("commission-correction-policy")) return ok({ ...policy, version: 2, enabled: true, audit_log_id: auditID });
      if (path.endsWith("/targets?limit=5&offset=2") && path.includes("commission-correction-plans")) return ok({ brand_id: brand, plan_id: planID, items: [planTarget()], total_count: "3", limit: 5, offset: 2 });
      if (path.endsWith("/targets?limit=5&offset=2")) return ok({ brand_id: brand, execution_id: executionID, items: [executionTarget()], total_count: "3", limit: 5, offset: 2 });
      if (path.endsWith("/retry") && path.includes("commission-correction-plans")) return ok(plan({ version: 3, state: "planning", last_error_code: null }));
      if (path.endsWith("/approve")) return ok(execution({ state: "applying", version: 3, approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }));
      if (path.endsWith("/continue") || path.endsWith("/retry")) return ok(execution({ state: "applying", version: 3, payout_mode: "automatic", approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }));
      if (path.includes("commission-correction-plans") && path.includes("?")) return ok({ brand_id: brand, items: [plan()], total_count: "1", limit: 20, offset: 0 });
      if (path.includes("commission-correction-executions") && path.includes("?")) return ok({ brand_id: brand, items: [execution()], total_count: "1", limit: 20, offset: 0 });
      if (path.includes("commission-correction-plans")) return ok(plan());
      return ok(execution());
    });
    const api = withActor(fetcher);
    await expect(api.getPolicy(brand)).resolves.toMatchObject({ enabled: false });
    await expect(api.updatePolicy(brand, { version: 1, enabled: true, reason: "Enable approved policy" }, "policy-key-0001")).resolves.toMatchObject({ version: 2 });
    await expect(api.listPlans(brand)).resolves.toMatchObject({ total_count: "1" });
    await expect(api.readPlan(brand, planID)).resolves.toMatchObject({ id: planID });
    await expect(api.listPlanTargets(brand, planID, 5, 2)).resolves.toMatchObject({ plan_id: planID, offset: 2 });
    await expect(api.retryPlan(brand, planID, body, "plan-retry-key-01")).resolves.toMatchObject({ state: "planning", version: 3 });
    await expect(api.listExecutions(brand)).resolves.toMatchObject({ total_count: "1" });
    await expect(api.readExecution(brand, executionID)).resolves.toMatchObject({ id: executionID });
    await expect(api.listExecutionTargets(brand, executionID, 5, 2)).resolves.toMatchObject({ execution_id: executionID, offset: 2 });
    await expect(api.approve(brand, executionID, body, "approve-key-0001")).resolves.toMatchObject({ state: "applying", approved_by: actor });
    await expect(api.continue(brand, executionID, body, "continue-key-01")).resolves.toMatchObject({ state: "applying" });
    await expect(api.retry(brand, executionID, body, "execute-retry-01")).resolves.toMatchObject({ state: "applying" });
    expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual([
      "/api/v1/admin/commission-correction-policy", "/api/v1/admin/commission-correction-policy", "/api/v1/admin/commission-correction-plans?limit=20&offset=0",
      `/api/v1/admin/commission-correction-plans/${planID}`, `/api/v1/admin/commission-correction-plans/${planID}/targets?limit=5&offset=2`,
      `/api/v1/admin/commission-correction-plans/${planID}/retry`, "/api/v1/admin/commission-correction-executions?limit=20&offset=0",
      `/api/v1/admin/commission-correction-executions/${executionID}`, `/api/v1/admin/commission-correction-executions/${executionID}/targets?limit=5&offset=2`,
      `/api/v1/admin/commission-correction-executions/${executionID}/approve`, `/api/v1/admin/commission-correction-executions/${executionID}/continue`,
      `/api/v1/admin/commission-correction-executions/${executionID}/retry`,
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
    const writes = fetcher.mock.calls.filter(([, init]) => init?.method === "PUT" || init?.method === "POST");
    expect(writes.map(([, init]) => [init?.method, new Headers(init?.headers).get("Idempotency-Key"), new Headers(init?.headers).get("X-Commission-Correction-Actor-ID"), init?.body])).toEqual([
      ["PUT", "policy-key-0001", actor, '{"version":1,"enabled":true,"reason":"Enable approved policy"}'],
      ["POST", "plan-retry-key-01", actor, '{"version":2,"reason":"Reviewed exact correction"}'],
      ["POST", "approve-key-0001", actor, '{"version":2,"reason":"Reviewed exact correction"}'],
      ["POST", "continue-key-01", actor, '{"version":2,"reason":"Reviewed exact correction"}'],
      ["POST", "execute-retry-01", actor, '{"version":2,"reason":"Reviewed exact correction"}'],
    ]);
  });

  it("accepts only the version-one disabled blank-audit policy initialization shape", async () => {
    const validDefault = { brand_id: brand, version: 1, enabled: false, audit_log_id: "", updated_at: now };
    await expect(withActor(async () => ok(validDefault)).getPolicy(brand)).resolves.toMatchObject(validDefault);
    for (const bad of [
      { ...validDefault, enabled: true },
      { ...validDefault, audit_log_id: auditID },
      { ...validDefault, version: 2 },
      { ...validDefault, version: 2, audit_log_id: auditID, extra: null },
    ]) await expect(withActor(async () => ok(bad)).getPolicy(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("validates exact BigInt economics, including values beyond safe integers", async () => {
    await expect(withActor(async () => ok(execution())).readExecution(brand, executionID)).resolves.toMatchObject({ credit_points: "9223372036854775808", net_points: "9223372036854775801", approved_by: null, paused_plan_target_id: null });
    await expect(withActor(async () => ok({ brand_id: brand, plan_id: planID, items: [planTarget({ original_target_id: null, adjustment_version: null, earning_id: memberID, points_before: "0", points_after: "20", delta_points: "20" })], total_count: "1", limit: 20, offset: 0 })).listPlanTargets(brand, planID)).resolves.toMatchObject({ items: [{ original_target_id: null, adjustment_version: null, previous_correction_target_id: null, financial_version: null, earning_id: memberID }] });
    await expect(withActor(async () => ok({ brand_id: brand, execution_id: executionID, items: [executionTarget()], total_count: "1", limit: 20, offset: 0 })).listExecutionTargets(brand, executionID)).resolves.toMatchObject({ items: [{ state: "pending", ledger_entry_id: null, financial_version: null }] });
    const badPlans = [
      plan({ net_points: "19" }), plan({ credit_points: "020" }), plan({ credit_points: null }),
      plan({ planned_count: "2" }), plan({ state: "failed", last_error_code: null }),
      plan({ calculated_points: "9223372036854775808" }), plan({ updated_at: "2026-02-30T00:00:00Z" }),
      plan({ extra_field: true } as Partial<CorrectionPlan>), plan({ brand_id: otherBrand }),
      plan({ state: "planning", credit_points: null, debit_points: null, net_points: null, planned_count: "0" }),
      { ...plan(), state: "blocked", last_error_code: "COMMISSION_CORRECTION_STATE_CONFLICT" } as unknown as CorrectionPlan,
    ];
    for (const value of badPlans) await expect(withActor(async () => ok(value)).readPlan(brand, planID)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const badExecutions = [
      execution({ net_points: "9223372036854775800" }), execution({ state: "awaiting_approval", payout_mode: "automatic" }),
      execution({ state: "completed", applied_count: "1" }), execution({ applied_credit_points: "1" }),
      execution({ target_count: "100001" }), execution({ state: "paused", last_error_code: null }),
      execution({ evidence_epoch: "9223372036854775808" }),
      execution({ state: "stale", approval_actor_type: "admin" }),
      execution({ state: "stale", payout_mode: "automatic" }),
      execution({ extra_field: true } as Partial<CorrectionExecution>), execution({ brand_id: otherBrand }),
    ];
    for (const value of badExecutions) await expect(withActor(async () => ok(value)).readExecution(brand, executionID)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "completed", applied_count: "1", applied_credit_points: "9223372036854775808", applied_debit_points: "7", approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).readExecution(brand, executionID))
      .resolves.toMatchObject({ state: "completed" });
    await expect(withActor(async () => ok(execution({ state: "stale", last_error_code: null }))).readExecution(brand, executionID)).resolves.toMatchObject({ state: "stale", approved_by: null });
  });

  it("enforces target delta, nullable financial witness, zero-delta ledger, and parent binding", async () => {
    const invalidTargets = [
      planTarget({ delta_points: "19" }), planTarget({ plan_id: otherBrand }), planTarget({ extra_field: 1 } as Partial<CorrectionPlanTarget>),
      planTarget({ original_target_id: null }), planTarget({ adjustment_version: null }),
      planTarget({ previous_correction_target_id: auditID }), planTarget({ financial_version: 3 }),
      planTarget({ original_target_id: null, adjustment_version: null, previous_correction_target_id: null, financial_version: null, earning_id: null }),
      planTarget({ original_target_id: null, adjustment_version: null, previous_correction_target_id: null, financial_version: null, points_before: "0" }),
    ];
    for (const value of invalidTargets) await expect(withActor(async () => ok({ brand_id: brand, plan_id: planID, items: [value], total_count: "1", limit: 20, offset: 0 })).listPlanTargets(brand, planID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const dangling = executionTarget({ ledger_entry_id: auditID });
    await expect(withActor(async () => ok({ brand_id: brand, execution_id: executionID, items: [dangling], total_count: "1", limit: 20, offset: 0 })).listExecutionTargets(brand, executionID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const appliedNoop = executionTarget({ points_before: "7", points_after: "7", delta_points: "0", state: "applied", audit_log_id: auditID, financial_version: 9, applied_at: now });
    await expect(withActor(async () => ok({ brand_id: brand, execution_id: executionID, items: [appliedNoop], total_count: "1", limit: 20, offset: 0 })).listExecutionTargets(brand, executionID)).resolves.toMatchObject({ items: [{ ledger_entry_id: null, state: "applied" }] });
    await expect(withActor(async () => ok({ brand_id: otherBrand, plan_id: planID, items: [], total_count: "0", limit: 20, offset: 0 })).listPlanTargets(brand, planID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok({ brand_id: brand, plan_id: otherBrand, items: [], total_count: "0", limit: 20, offset: 0 })).listPlanTargets(brand, planID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("accepts mixed positive and negative plan deltas and checks all aggregate equations", async () => {
    const mixedPlan = plan({ before_points: "900719925474099312345", calculated_points: "900719925474099312345", credit_points: "100", debit_points: "100", net_points: "0", target_count: "2", planned_count: "2" });
    await expect(withActor(async () => ok(mixedPlan)).readPlan(brand, planID)).resolves.toMatchObject({ credit_points: "100", debit_points: "100", net_points: "0" });
    for (const value of [
      plan({ before_points: "1000", calculated_points: "1000", credit_points: "99", debit_points: "100", net_points: "0" }),
      plan({ before_points: "1000", calculated_points: "1000", credit_points: "100", debit_points: "100", net_points: "1" }),
      plan({ evidence_epoch: "9223372036854775808" }),
    ]) await expect(withActor(async () => ok(value)).readPlan(brand, planID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("validates exact pagination, safe versions, operation states, and original idempotent receipts", async () => {
    await expect(withActor(async () => ok({ brand_id: brand, items: [plan(), plan()], total_count: "2", limit: 20, offset: 0 })).listPlans(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok({ brand_id: brand, items: [plan()], total_count: "1", limit: 19, offset: 0 })).listPlans(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok({ brand_id: brand, items: [], total_count: "0", limit: 20, offset: 40 })).listPlans(brand, 20, 40)).resolves.toMatchObject({ items: [], offset: 40 });
    await expect(withActor(async () => ok({ brand_id: brand, items: [], total_count: "5", limit: 20, offset: 0 })).listPlans(brand)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(plan({ version: Number.MAX_SAFE_INTEGER + 1 }))).readPlan(brand, planID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(plan({ version: 4, state: "planning", last_error_code: null }))).retryPlan(brand, planID, body, "plan-retry-key-01")).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "paused", version: 3, last_error_code: "COMMISSION_CORRECTION_CYCLE_HELD", cycle_hold_active: true, approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).approve(brand, executionID, body, "approve-key-0001")).resolves.toMatchObject({ state: "paused", cycle_hold_active: true });
    await expect(withActor(async () => ok(execution({ state: "paused", version: 3, last_error_code: "COMMISSION_CORRECTION_CYCLE_HELD", cycle_hold_active: true }))).readExecution(brand, executionID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "paused", version: 3, last_error_code: "COMMISSION_CORRECTION_CYCLE_HELD", paused_plan_target_id: targetID, cycle_hold_active: true, approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).readExecution(brand, executionID)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "applying", version: 3, approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).continue(brand, executionID, body, "continue-key-01")).resolves.toMatchObject({ state: "applying" });
    await expect(withActor(async () => ok(execution({ state: "completed", version: 3, applied_count: "1", applied_credit_points: "9223372036854775808", applied_debit_points: "7", approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).retry(brand, executionID, body, "execute-retry-01")).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "applying", version: 3, payout_mode: "automatic", approved_by: actor, approval_actor_type: "admin", approval_audit_log_id: auditID }))).approve(brand, executionID, body, "approve-key-0001")).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    await expect(withActor(async () => ok(execution({ state: "applying", version: 3, approved_by: otherBrand, approval_actor_type: "admin", approval_audit_log_id: auditID }))).approve(brand, executionID, body, "approve-key-0001")).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("captures actor identity once and rejects invalid writes before fetch", async () => {
    let mutableActor = actor;
    const fetcher = vi.fn<typeof fetch>(async () => ok({ ...plan(), version: 3, state: "planning", last_error_code: null }));
    const api = createCommissionCorrectionsApi(fetcher, mutableActor);
    mutableActor = otherBrand;
    await api.retryPlan(brand, planID, body, "plan-retry-key-01");
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("X-Commission-Correction-Actor-ID")).toBe(actor);
    await expect(createCommissionCorrectionsApi(fetcher).retryPlan(brand, planID, body, "plan-retry-key-01")).rejects.toBeInstanceOf(AdminApiError);
    const initialCalls = fetcher.mock.calls.length;
    await expect(withActor(fetcher).approve(brand, executionID, { ...body, ignored: true } as typeof body, "approve-key-0001")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(withActor(fetcher).readExecution(otherBrand, "bad-id")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher.mock.calls).toHaveLength(initialCalls);
  });
});
