import { describe, expect, it, vi } from "vitest";
import { reactive } from "vue";
import type { AdminAccount } from "./admin-api";
import { correctionPermissions, createCorrectionApi, type Correction, type CorrectionBody, type CorrectionExpected } from "./correction-api";

const brand = "11111111-1111-4111-8111-111111111111";
const game = "22222222-2222-4222-8222-222222222222";
const period = "33333333-3333-4333-8333-333333333333";
const drawId = "44444444-4444-4444-8444-444444444444";
const correctionId = "55555555-5555-4555-8555-555555555555";
const actor = "66666666-6666-4666-8666-666666666666";
const newDrawId = "77777777-7777-4777-8777-777777777777";
const jobId = "88888888-8888-4888-8888-888888888888";
const iso = "2026-10-06T01:00:00Z";
const huge = "900719925474099312345";
const context = (financial = false) => ({
  brand_id: brand, game_id: game, period_id: period, period_version: 9, period_status: financial ? "settling" : "drawn",
  draw_result_id: drawId, draw: { regular: [1, 4], special: [7], digits: [] },
  model: { model: "X_PLUS_Y", ordered: false }, current_job_id: financial ? jobId : null,
  current_job_version: financial ? 2 : null, policy_version: 3, mode: financial ? "automatic" : null,
  requires_resettlement: financial, can_correct: true,
});
const correction = (overrides: Partial<Correction> = {}): Correction => ({
  id: correctionId, brand_id: brand, game_id: game, period_id: period, previous_draw_result_id: drawId,
  draw_result_id: newDrawId, result: { regular: [2, 5], special: [8], digits: [] },
  period_version: 10, previous_job_id: null, new_job_id: null, policy_version: null, mode: null, state: "completed", version: 1,
  target_count: 0, created_by: actor, reason: "verified source", created_at: iso, completed_at: iso,
  last_error_code: null, pending_count: 0, reversed_count: 0, unchanged_count: 0, excluded_count: 0, failed_count: 0,
  reverse_points: "0", reversed_points: "0", can_retry: false,
  new_job_state: null, new_job_version: null, new_job_error_code: null, ...overrides,
});
const financial = (overrides: Partial<Correction> = {}) => correction({
  previous_job_id: jobId, policy_version: 3, mode: "automatic", state: "reversing",
  target_count: 2, pending_count: 2, reverse_points: huge, completed_at: null, ...overrides,
});
const body = (withJob = false): CorrectionBody => ({
  version: 9, policy_version: withJob ? 3 : null, result: { regular: [2, 5], special: [8], digits: [] }, reason: "verified source",
});
const expected = (withJob = false): CorrectionExpected => ({ periodId: period, accountId: actor, mode: withJob ? "automatic" : null });
const ok = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } });
const errorResponse = (status: number) => new Response(JSON.stringify({ success: false, error: { code: "ERR", message: "denied" } }), { status });
const account = (overrides: Partial<AdminAccount> = {}): AdminAccount => ({
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["draw.view.brand", "draw.correct.brand", "draw.correction_retry.brand", "settlement.run.brand"] }, ...overrides,
});

describe("correction permissions", () => {
  it("requires scoped grants and denies every write to super admins", () => {
    expect(correctionPermissions(account(), brand)).toEqual({ view: true, correct: true, retry: true, settleRun: true });
    expect(correctionPermissions(account({ super_admin: true, platform_permissions: ["draw.view.platform"] }), brand))
      .toEqual({ view: true, correct: false, retry: false, settleRun: false });
    expect(correctionPermissions(account(), "")).toEqual({ view: false, correct: false, retry: false, settleRun: false });
    expect(correctionPermissions(account({ brand_ids: [] }), brand)).toEqual({ view: false, correct: false, retry: false, settleRun: false });
    expect(correctionPermissions(account({ permissions_by_brand: { [game]: ["draw.view.brand", "draw.correct.brand"] } }), brand))
      .toEqual({ view: false, correct: false, retry: false, settleRun: false });
    expect(correctionPermissions(account({ permissions_by_brand: { [brand]: ["draw.correct.brand"] } }), brand))
      .toEqual({ view: false, correct: true, retry: false, settleRun: false });
  });
});

describe("create correction historical receipts", () => {
  it("posts to the frozen draw ID exactly once and returns the original 201 receipt without GET", async () => {
    const receipt = correction();
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(receipt, 201));
    expect(await createCorrectionApi(fetcher).create(brand, drawId, body(), "key-1", expected())).toEqual(receipt);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/draw-results/" + drawId + "/correct");
    const init = fetcher.mock.calls[0][1] as RequestInit;
    expect(init).toMatchObject({ method: "POST", credentials: "include", body: JSON.stringify(body()) });
    expect(new Headers(init.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init.headers).get("Idempotency-Key")).toBe("key-1");
  });

  it("returns original financial reversing/v1 and arbitrary precision strings", async () => {
    const receipt = financial();
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(receipt, 201));
    const result = await createCorrectionApi(fetcher).create(brand, drawId, body(true), "k", expected(true));
    expect(result).toEqual(receipt);
    expect(typeof result.reverse_points).toBe("string");
    expect(result.reverse_points).toBe(huge);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("preserves already canonical arrays including ordered digits and accepts Vue proxies", async () => {
    const input = reactive(body());
    input.result = { regular: [5, 2], special: [9, 8], digits: [0, 9, 2] };
    const result = { regular: [5, 2], special: [9, 8], digits: [0, 9, 2] };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(correction({ result }), 201));
    expect((await createCorrectionApi(fetcher).create(brand, drawId, input, "k", expected())).result).toEqual(result);
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body)).result).toEqual(result);
  });

  it.each([
    ["period", { period_id: game }],
    ["actor", { created_by: game }],
    ["brand", { brand_id: game }],
    ["draw", { previous_draw_result_id: newDrawId }],
    ["period version before start", { period_version: 9 }],
    ["period version too far", { period_version: 11 }],
    ["correction version", { version: 2 }],
    ["reason", { reason: "other reason" }],
    ["result numbers", { result: { regular: [2, 6], special: [8], digits: [] } }],
    ["result order", { result: { regular: [5, 2], special: [8], digits: [] } }],
    ["result missing array", { result: { regular: [2, 5], special: [8] } }],
    ["no-job state", { state: "reversing" }],
    ["no-job prior job", { previous_job_id: jobId }],
    ["no-job mode", { mode: "automatic" }],
    ["policy", { policy_version: 3 }],
    ["already published job", { new_job_id: jobId }],
    ["nested job state", { new_job_state: "completed" }],
    ["counts", { target_count: 1 }],
    ["noncanonical points", { reverse_points: "01" }],
    ["numeric points", { reverse_points: 123 }],
    ["invalid timestamp", { created_at: "2026-02-30T01:00:00Z" }],
  ])("rejects wrong successful create receipt: %s as unknown 502", async (_name, changes) => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ ...correction(), ...changes }, 201));
    await expect(createCorrectionApi(fetcher).create(brand, drawId, body(), "k", expected())).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it.each([
    { state: "resettling" }, { mode: null }, { mode: "manual" }, { policy_version: 4 }, { previous_job_id: null },
  ])("rejects mismatched financial receipt %j", async changes => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ ...financial(), ...changes }, 201));
    await expect(createCorrectionApi(fetcher).create(brand, drawId, body(true), "k", expected(true))).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("rejects unconfigured financial expected mode and incomplete inputs before fetch", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createCorrectionApi(fetcher);
    await expect(api.create(brand, drawId, body(true), "k", expected())).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.create(brand, drawId, { ...body(), result: { regular: [], special: [] } } as unknown as CorrectionBody, "k")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.create(brand, drawId, { ...body(), version: Number.MAX_SAFE_INTEGER }, "k")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("allows omitted expected scope without any read and preserves caller supplied keys across requests", async () => {
    const fetcher = vi.fn<typeof fetch>().mockRejectedValueOnce(new Error("lost response")).mockResolvedValueOnce(ok(correction(), 201)).mockResolvedValueOnce(ok(correction(), 201));
    const api = createCorrectionApi(fetcher);
    await expect(api.create(brand, drawId, body(), "frozen-key")).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
    await api.create(brand, drawId, body(), "frozen-key");
    await api.create(brand, drawId, body(), "new-key");
    expect(fetcher).toHaveBeenCalledTimes(3);
    expect(fetcher.mock.calls[0][1]?.body).toBe(fetcher.mock.calls[1][1]?.body);
    expect(fetcher.mock.calls.map(c => new Headers(c[1]?.headers).get("Idempotency-Key"))).toEqual(["frozen-key", "frozen-key", "new-key"]);
    expect(fetcher.mock.calls.every(c => c[1]?.method === "POST")).toBe(true);
  });
});

describe("retry correction historical receipts", () => {
  it("returns the original 200 reversing receipt with original creator/reason and no read", async () => {
    const receipt = financial({ version: 5, created_by: game, reason: "original reason" });
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(receipt));
    expect(await createCorrectionApi(fetcher).retry(brand, correctionId, { version: 4, reason: "retry audit reason" }, "retry-key")).toEqual(receipt);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/corrections/" + correctionId + "/retry");
    expect(fetcher.mock.calls[0][1]?.body).toBe(JSON.stringify({ version: 4, reason: "retry audit reason" }));
  });

  it.each([{ version: 4 }, { version: 6 }, { state: "resettling" }, { id: game }, { brand_id: game }])("rejects invalid retry receipt %j", async changes => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok({ ...financial({ version: 5 }), ...changes }));
    await expect(createCorrectionApi(fetcher).retry(brand, correctionId, { version: 4, reason: "retry audit reason" }, "k")).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });
});

describe("correction reads and HTTP responses", () => {
  it("accepts nullable unconfigured context and legitimate uncorrectable context", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(context()))
      .mockResolvedValueOnce(ok({ ...context(), period_status: "waiting_draw", draw: null, draw_result_id: null, can_correct: false }));
    const api = createCorrectionApi(fetcher);
    expect((await api.context(brand, period)).mode).toBeNull();
    expect((await api.context(brand, period)).can_correct).toBe(false);
    expect(fetcher.mock.calls.every(c => c[1]?.credentials === "include" && c[1]?.method === "GET")).toBe(true);
  });

  it("checks context and detail scopes", async () => {
    for (const ctx of [{ ...context(), brand_id: game }, { ...context(), period_id: game }, { ...context(), requires_resettlement: true }]) {
      await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(ok(ctx))).context(brand, period)).rejects.toMatchObject({ status: 502 });
    }
    for (const receipt of [correction({ brand_id: game }), correction({ id: game })]) {
      await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(ok(receipt))).detail(brand, correctionId)).rejects.toMatchObject({ status: 502 });
    }
  });

  it("validates history page metadata and item scope", async () => {
    const page = { brand_id: brand, period_id: period, items: [correction()], limit: 20, offset: 40, has_more: true };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(page));
    expect(await createCorrectionApi(fetcher).history(brand, period, 20, 40)).toEqual(page);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/periods/" + period + "/corrections?limit=20&offset=40");
    for (const invalid of [{ ...page, limit: 10 }, { ...page, offset: 0 }, { ...page, period_id: game }, { ...page, items: [correction({ period_id: game })] }]) {
      await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(ok(invalid))).history(brand, period, 20, 40)).rejects.toMatchObject({ status: 502 });
    }
  });

  it("validates target int64 strings without loss of precision and page scope", async () => {
    const target = { order_id: period, member_id: actor, state: "reversed", version: 2, old_order_version: 1, old_order_status: "won",
      old_calculation_id: game, old_payout_entry_id: drawId, old_prize_points: "9223372036854775807", reversal_entry_id: newDrawId, reset_order_version: 2, error_code: null };
    const page = { brand_id: brand, correction_id: correctionId, items: [target], limit: 20, offset: 0, has_more: false };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(ok(page));
    expect((await createCorrectionApi(fetcher).targets(brand, correctionId)).items[0].old_prize_points).toBe("9223372036854775807");
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/corrections/" + correctionId + "/targets?limit=20&offset=0");
    for (const invalid of [{ ...page, correction_id: game }, { ...page, items: [{ ...target, old_prize_points: "9223372036854775808" }] }]) {
      await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(ok(invalid))).targets(brand, correctionId)).rejects.toMatchObject({ status: 502 });
    }
  });

  it("rejects pagination errors before requests", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createCorrectionApi(fetcher);
    for (const [limit, offset] of [[0, 0], [101, 0], [1, -1], [20, 1_000_001], [1.5, 0]]) {
      await expect(api.history(brand, period, limit, offset)).rejects.toMatchObject({ code: "INVALID_INPUT" });
      await expect(api.targets(brand, correctionId, limit, offset)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    }
    expect(fetcher).not.toHaveBeenCalled();
  });

  it.each([400, 401, 403, 404, 409, 422, 429, 502, 503])("preserves HTTP %s on write without internal reads", async status => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(errorResponse(status));
    await expect(createCorrectionApi(fetcher).create(brand, drawId, body(), "k", expected())).rejects.toMatchObject({ status, code: "ERR" });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("maps malformed or wrong successful HTTP replies to unknown 502", async () => {
    for (const response of [ok(correction(), 200), ok({}, 201), new Response("not JSON", { status: 201 }), new Response(JSON.stringify({ success: false }), { status: 201 })]) {
      await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(response)).create(brand, drawId, body(), "k", expected())).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createCorrectionApi(vi.fn<typeof fetch>().mockResolvedValueOnce(ok(financial({ version: 5 }), 201))).retry(brand, correctionId, { version: 4, reason: "retry" }, "k")).rejects.toMatchObject({ status: 502 });
  });
});
