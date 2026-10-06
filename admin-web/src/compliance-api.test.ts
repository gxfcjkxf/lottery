import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { compliancePermissions, createCompliancePolicyApi, validComplianceConfig, validComplianceReason, type ComplianceConfig, type ComplianceDecision, type ComplianceGateRecord, type CompliancePolicy } from "./compliance-api";

const brand = "00000000-0000-4000-8000-000000000001", other = "00000000-0000-4000-8000-000000000002", actor = "00000000-0000-4000-8000-000000000003", audit = "00000000-0000-4000-8000-000000000004";
const config: ComplianceConfig = { age_enabled: false, minimum_age: null, region_enabled: false, allowed_countries: [], identity_enabled: false };
const at = "2026-10-07T03:04:05Z";
const policy: CompliancePolicy = { brand_id: brand, version: 1, config, updated_at: at };
function decision(overrides: Partial<ComplianceDecision> = {}): ComplianceDecision { return { id: actor, brand_id: brand, policy_version: 1, config, operation: "registration", decision: "allow", checks: [
  { check: "age", enabled: false, decision: "allow", reason_code: "CHECK_DISABLED" },
  { check: "region", enabled: false, decision: "allow", reason_code: "CHECK_DISABLED" },
  { check: "identity", enabled: false, decision: "allow", reason_code: "CHECK_DISABLED" },
], adapter_mode: "stub", created_by: actor, reason: "Explicit test", audit_log_id: audit, created_at: at, ...overrides }; }
const gatedConfig: ComplianceConfig = { ...config, identity_enabled: true };
function gate(overrides: Partial<ComplianceGateRecord> = {}): ComplianceGateRecord { return { id: actor, brand_id: brand, policy_version: 2, config: gatedConfig, operation: "registration", action: "register", decision: "review", checks: [
  { check: "age", enabled: false, decision: "allow", reason_code: "CHECK_DISABLED" },
  { check: "region", enabled: false, decision: "allow", reason_code: "CHECK_DISABLED" },
  { check: "identity", enabled: true, decision: "review", reason_code: "ADAPTER_NOT_CONFIGURED" },
], adapter_mode: "stub", actor_type: "anonymous", actor_id: null, member_id: null, request_id: "request-123", audit_log_id: audit, created_at: at, ...overrides }; }
function response(data: unknown, status = 200): Response { return new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } }); }
function account(overrides: Partial<AdminAccount> = {}): AdminAccount { return { id: actor, super_admin: false, brand_ids: [brand], permissions: [], ...overrides }; }

describe("compliance permissions", () => {
  it("requires exact brand/platform grants and denies brand writes and runs to super-admins", () => {
    const grants = ["compliance_policy.view.brand", "compliance_policy.write.brand", "compliance_check.view.brand", "compliance_check.run.brand"];
    expect(compliancePermissions(account({ permissions_by_brand: { [brand]: grants } }), brand)).toEqual({ viewPolicy: true, writePolicy: true, viewChecks: true, runCheck: true });
    expect(compliancePermissions(account({ super_admin: true, permissions_by_brand: { [brand]: grants } }), brand)).toEqual({ viewPolicy: true, writePolicy: false, viewChecks: true, runCheck: false });
    expect(compliancePermissions(account({ brand_ids: [], permissions_by_brand: { [brand]: grants }, platform_permissions: ["compliance_check.view.platform"] }), brand)).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: true, runCheck: false });
    expect(compliancePermissions(account({ permissions_by_brand: { [other]: grants }, permissions: grants }), brand)).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: false, runCheck: false });
    expect(compliancePermissions(account({ permissions: grants }), brand)).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: false, runCheck: false });
    expect(compliancePermissions(account({ platform_permissions: ["compliance_policy.write.platform", "compliance_check.run.platform"] }), brand)).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: false, runCheck: false });
    expect(compliancePermissions(account({ super_admin: true, platform_permissions: ["compliance_policy.view.platform", "compliance_check.view.platform"] }), brand)).toEqual({ viewPolicy: true, writePolicy: false, viewChecks: true, runCheck: false });
    expect(compliancePermissions(account({ id: "bad", platform_permissions: ["compliance_policy.view.platform", "compliance_check.view.platform"] }), brand)).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: false, runCheck: false });
    expect(compliancePermissions(account({ platform_permissions: ["compliance_policy.view.platform", "compliance_check.view.platform"] }), "bad")).toEqual({ viewPolicy: false, writePolicy: false, viewChecks: false, runCheck: false });
  });
});

describe("compliance API contracts", () => {
  it("validates exact policy configuration and strict country normalization", () => {
    expect(validComplianceConfig(config)).toBe(true);
    expect(validComplianceConfig({ ...config, age_enabled: true })).toBe(false);
    expect(validComplianceConfig({ ...config, age_enabled: true, minimum_age: 18 })).toBe(true);
    for (const minimum_age of [17, 121, 18.5, NaN]) expect(validComplianceConfig({ ...config, minimum_age })).toBe(false);
    for (const allowed_countries of [["US", "CA"], ["CA", "CA"], ["ca"], ["USA"], Array.from({ length: 251 }, (_, i) => String.fromCharCode(65 + i % 26) + String.fromCharCode(65 + Math.floor(i / 26)))]) expect(validComplianceConfig({ ...config, allowed_countries })).toBe(false);
    expect(validComplianceConfig({ ...config, region_enabled: true })).toBe(false);
    expect(validComplianceConfig({ ...config, region_enabled: true, allowed_countries: ["CA", "US"] })).toBe(true);
    expect(validComplianceConfig({ ...config, ignored: true })).toBe(false);
    expect(validComplianceReason("A reason")).toBe(true);
    for (const reason of ["", " ", " padded ", " padded", "padded ", "line\nbreak", "control\u0085"]) expect(validComplianceReason(reason)).toBe(false);
    expect(validComplianceReason("é".repeat(250))).toBe(true);
    expect(validComplianceReason("é".repeat(251))).toBe(false);
  });

  it("sends brand scope, exact PUT body, and original idempotency key; validates receipt", async () => {
    const body = { version: 1, config: { ...config, age_enabled: true, minimum_age: 21 }, reason: "Enable age check" }, key = "compliance-policy-key-01";
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ ...policy, version: 2, config: body.config, audit_log_id: audit }));
    await expect(createCompliancePolicyApi(fetcher).put(brand, body, key)).resolves.toEqual({ ...policy, version: 2, config: body.config, audit_log_id: audit });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/compliance-policy"); expect(init?.method).toBe("PUT"); expect(init?.credentials).toBe("same-origin");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand); expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(key);
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...policy, version: 3, audit_log_id: audit }))).put(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).put(brand, { ...body, reason: "bad\nreason" }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).put(brand, { ...body, reason: "x".repeat(501) }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
    for (const reason of [" padded", "padded "]) await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).put(brand, { ...body, reason }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
  });

  it("enforces POST 201, exact stub decision shape, and stable request key", async () => {
    const body = { version: 1, operation: "registration" as const, reason: "Explicit test" }, key = "compliance-check-key-01", fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(decision(), 201));
    await expect(createCompliancePolicyApi(fetcher).run(brand, body, key)).resolves.toEqual(decision());
    const [url, init] = fetcher.mock.calls[0]; expect(url).toBe("/api/v1/admin/compliance-checks"); expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(body); expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand); expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(key);
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response(decision(), 200))).run(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response(decision({ checks: [decision().checks[1], decision().checks[0], decision().checks[2]] }), 201))).run(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    const enabled = decision({ config: { ...config, age_enabled: true, minimum_age: 18 }, checks: [{ check: "age", enabled: true, decision: "review", reason_code: "ADAPTER_NOT_CONFIGURED" }, decision().checks[1], decision().checks[2]], decision: "review" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response(enabled, 201))).run(brand, body, key)).resolves.toEqual(enabled);
    const contradictory = { ...enabled, config };
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response(contradictory, 201))).run(brand, body, key)).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    for (const reason of [" leading", "trailing "]) await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).run(brand, { ...body, reason }, key)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
  });

  it("validates policy/history/decision pages and strict paging", async () => {
    const revision = { id: actor, brand_id: brand, version: 1, config, changed_by: null, reason: "Initial system policy", audit_log_id: null, created_at: at };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(response(policy))
      .mockResolvedValueOnce(response({ brand_id: brand, items: [revision], limit: 20, offset: 0, total_count: "1" }))
      .mockResolvedValueOnce(response({ brand_id: brand, operation: "registration", items: [decision()], limit: 20, offset: 0, total_count: "1" }));
    const api = createCompliancePolicyApi(fetcher);
    await expect(api.get(brand)).resolves.toEqual(policy);
    await expect(api.history(brand)).resolves.toMatchObject({ brand_id: brand, items: [revision], total_count: "1" });
    await expect(api.decisions(brand, 20, 0, "registration")).resolves.toMatchObject({ operation: "registration", items: [decision()] });
    expect(fetcher.mock.calls.map(([url]) => url)).toEqual(["/api/v1/admin/compliance-policy", "/api/v1/admin/compliance-policy/history?limit=20&offset=0", "/api/v1/admin/compliance-checks?limit=20&offset=0&operation=registration"]);
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ ...policy, brand_id: other }))).get(brand)).rejects.toMatchObject({ status: 502 });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).history(brand, 0, 0)).rejects.toBeInstanceOf(AdminApiError);
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, items: [], limit: 20, offset: 0, total_count: "01" }))).history(brand)).rejects.toMatchObject({ status: 502 });
    const historyResult = (item: unknown) => createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, items: [item], limit: 20, offset: 0, total_count: "1" }))).history(brand);
    for (const bad of [
      { ...revision, changed_by: actor },
      { ...revision, audit_log_id: audit },
      { ...revision, version: 2 },
      { ...revision, version: 2, changed_by: actor },
    ]) await expect(historyResult(bad)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const later = { ...revision, version: 2, changed_by: actor, audit_log_id: audit };
    await expect(historyResult(later)).resolves.toMatchObject({ items: [later] });
  });

  it("reads strictly scoped live gate history with exact operation filters and validates each rejection receipt", async () => {
    const item = gate(), fetcher = vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, operation: "registration", items: [item], limit: 10, offset: 20, total_count: "21" }));
    await expect(createCompliancePolicyApi(fetcher).gates(brand, 10, 20, "registration")).resolves.toMatchObject({ brand_id: brand, operation: "registration", items: [item], limit: 10, offset: 20, total_count: "21" });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/compliance-gates?limit=10&offset=20&operation=registration");
    expect(init?.method).toBe("GET"); expect(init?.credentials).toBe("same-origin"); expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    const page = (row: unknown, brandId = brand, operation: "registration" | "betting" | null = null) => createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brandId, operation, items: [row], limit: 20, offset: 0, total_count: "1" }))).gates(brand);
    const betting = gate({ operation: "betting", action: "bet_preview", actor_type: "user", actor_id: actor, member_id: other });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, operation: "betting", items: [betting], limit: 20, offset: 0, total_count: "1" }))).gates(brand, 20, 0, "betting")).resolves.toMatchObject({ items: [betting] });
    for (const validActor of [
      gate({ action: "join", actor_type: "user", actor_id: actor }),
      gate({ action: "join", actor_type: "user", actor_id: actor, member_id: other }),
      gate({ action: "operator_join", actor_type: "admin", actor_id: actor }),
    ]) await expect(page(validActor)).resolves.toMatchObject({ items: [validActor] });

    for (const malformed of [
      { ...item, private_field: "must not pass" },
      { ...item, brand_id: other },
      { ...item, action: "bet_place" },
      { ...item, actor_id: actor },
      { ...item, request_id: "r".repeat(81) },
      { ...item, decision: "allow" },
      { ...item, checks: [item.checks[1], item.checks[0], item.checks[2]] },
      { ...item, config },
    ]) await expect(page(malformed)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: other, operation: null, items: [], limit: 20, offset: 0, total_count: "0" }))).gates(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, operation: null, items: [], limit: 20, offset: 0, total_count: "01" }))).gates(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>().mockResolvedValue(response({ brand_id: brand, operation: null, items: [], limit: 20, offset: 0, total_count: "0" }))).gates(brand, 20, 0, "registration")).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createCompliancePolicyApi(vi.fn<typeof fetch>()).gates(brand, 20, 0, "withdrawal" as never)).rejects.toMatchObject({ status: 400, code: "INVALID_INPUT" });
  });
});
