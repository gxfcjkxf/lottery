import { describe, expect, it, vi } from "vitest";
import {
  createRewardsApi,
  RewardsApiError,
  type RewardAction,
  type RewardActionPage,
  type RewardGrantBody,
  type RewardOrder,
  type RewardOrderPage,
} from "./rewards-api";

const brand = "0199a000-0000-7000-8000-000000000001";
const otherBrand = "0199a000-0000-7000-8000-000000000002";
const actor = "0199a000-0000-7000-8000-000000000003";
const member = "0199a000-0000-7000-8000-000000000004";
const orderID = "0199a000-0000-7000-8000-000000000005";
const grantLedger = "0199a000-0000-7000-8000-000000000006";
const revokeLedger = "0199a000-0000-7000-8000-000000000007";
const audit = "0199a000-0000-7000-8000-000000000008";
const actionID = "0199a000-0000-7000-8000-000000000009";
const now = "2026-10-08T10:00:00.123456Z";
const requestID = "reward-sdk-test-request-001";
const maxInt64 = "9223372036854775807";

function order(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return {
    id: orderID,
    brand_id: brand,
    member_id: member,
    points: "25",
    state: "granted",
    version: 1,
    grant_ledger_entry_id: grantLedger,
    revoke_ledger_entry_id: null,
    creation_audit_log_id: audit,
    last_audit_log_id: audit,
    last_error_code: null,
    created_by: actor,
    reason: "approved customer reward",
    point_policy_version: "1",
    created_at: now,
    updated_at: now,
    revoked_at: null,
    ...overrides,
  };
}

function pending(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return order({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT", ...overrides });
}

function revoked(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return order({ state: "revoked", version: 2, revoke_ledger_entry_id: revokeLedger, revoked_at: "2026-10-08T10:01:00Z", updated_at: "2026-10-08T10:01:00Z", ...overrides });
}

function action(overrides: Partial<RewardAction> = {}): RewardAction {
  return {
    id: actionID,
    brand_id: brand,
    order_id: orderID,
    version: 1,
    operation: "grant",
    state_before: null,
    state_after: "granted",
    actor_id: actor,
    reason: "approved customer reward",
    audit_log_id: audit,
    ledger_entry_id: grantLedger,
    created_at: now,
    ...overrides,
  };
}

function orderPage(items: RewardOrder[], overrides: Partial<RewardOrderPage> = {}): RewardOrderPage {
  return { brand_id: brand, items, total_count: String(items.length), limit: 20, offset: 0, ...overrides };
}

function actionPage(items: RewardAction[], overrides: Partial<RewardActionPage> = {}): RewardActionPage {
  return { brand_id: brand, order_id: orderID, items, total_count: String(items.length), limit: 20, offset: 0, ...overrides };
}

function ok(data: unknown, status = 200): Response {
  return new Response(JSON.stringify({ success: true, data, request_id: requestID }), { status });
}

function fail(code: string, status = 409): Response {
  return new Response(JSON.stringify({ success: false, error: { code, message: "safe server message" }, request_id: requestID }), { status });
}

describe("rewards API", () => {
  it("calls all six canonical routes with immutable actor, brand, idempotency and exact bodies", async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    const fetcher = vi.fn<typeof fetch>(async (input, init) => {
      const url = String(input);
      calls.push({ url, init });
      if (url.endsWith("/actions?limit=20&offset=0")) {
        return ok(actionPage([
          action({ id: actionID, version: 2, operation: "revoke", state_before: "granted", state_after: "revocation_pending", ledger_entry_id: null }),
          action({ id: "0199a000-0000-7000-8000-000000000010", version: 1 }),
        ], { total_count: "2" }));
      }
      if (url.endsWith("?limit=20&offset=0")) return ok(orderPage([order()]));
      if (url.endsWith("/retry-revocation")) return ok(revoked({ version: 3 }));
      if (url.endsWith("/revoke")) return ok(pending());
      if (url === "/api/v1/admin/reward-orders" && init?.method === "POST") return ok(order({ points: maxInt64 }), 201);
      return ok(order());
    });
    const api = createRewardsApi(fetcher, actor);
    const grantBody = Object.freeze({ member_id: member, points: maxInt64, reason: "approved maximum integer reward" });
    await expect(api.list(brand)).resolves.toMatchObject({ items: [{ id: orderID }], total_count: "1" });
    await expect(api.create(brand, grantBody, "reward-grant-key-0001")).resolves.toMatchObject({ points: maxInt64, state: "granted", version: 1 });
    await expect(api.read(brand, orderID)).resolves.toMatchObject({ id: orderID });
    await expect(api.actions(brand, orderID)).resolves.toMatchObject({ items: [{ version: 2 }, { version: 1 }] });
    await expect(api.revoke(brand, orderID, Object.freeze({ version: 1, reason: "customer asked for reversal" }), "reward-revoke-key-01"))
      .resolves.toMatchObject({ state: "revocation_pending", version: 2 });
    await expect(api.retryRevocation(brand, orderID, Object.freeze({ version: 2, reason: "retry after gift release" }), "reward-retry-key-001"))
      .resolves.toMatchObject({ state: "revoked", version: 3 });

    expect(calls.map(({ url, init }) => [url, init?.method ?? "GET"])).toEqual([
      ["/api/v1/admin/reward-orders?limit=20&offset=0", "GET"],
      ["/api/v1/admin/reward-orders", "POST"],
      [`/api/v1/admin/reward-orders/${orderID}`, "GET"],
      [`/api/v1/admin/reward-orders/${orderID}/actions?limit=20&offset=0`, "GET"],
      [`/api/v1/admin/reward-orders/${orderID}/revoke`, "POST"],
      [`/api/v1/admin/reward-orders/${orderID}/retry-revocation`, "POST"],
    ]);
    for (const { init } of calls) {
      const headers = new Headers(init?.headers);
      expect(headers.get("X-Brand-ID")).toBe(brand);
      expect(init?.credentials).toBe("include");
    }
    const writes = calls.filter(({ init }) => init?.method === "POST");
    expect(writes.map(({ init }) => [new Headers(init?.headers).get("X-Reward-Actor-ID"), new Headers(init?.headers).get("Idempotency-Key"), init?.body])).toEqual([
      [actor, "reward-grant-key-0001", JSON.stringify(grantBody)],
      [actor, "reward-revoke-key-01", JSON.stringify({ version: 1, reason: "customer asked for reversal" })],
      [actor, "reward-retry-key-001", JSON.stringify({ version: 2, reason: "retry after gift release" })],
    ]);
    expect(calls).toHaveLength(6); // Writes consume their receipts; there is no follow-up read.
  });

  it("rejects writes without a valid actor before fetch and does not treat actor as permission", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => ok(pending()));
    const noActor = createRewardsApi(fetcher);
    await expect(noActor.create(brand, { member_id: member, points: "1", reason: "valid reason" }, "reward-key-0001"))
      .rejects.toMatchObject({ code: "AUTH_ACTOR_REQUIRED", status: 0 });
    expect(fetcher).not.toHaveBeenCalled();

    const malformedActor = createRewardsApi(fetcher, "not-a-uuid");
    await expect(malformedActor.revoke(brand, orderID, { version: 1, reason: "valid reason" }, "reward-key-0002"))
      .rejects.toMatchObject({ code: "AUTH_ACTOR_REQUIRED", status: 0 });
    expect(fetcher).not.toHaveBeenCalled();

    // A syntactically valid actor is forwarded as context only; this client has
    // no local role/permission grant and lets the API remain authoritative.
    const validActor = createRewardsApi(fetcher, actor);
    await expect(validActor.revoke(brand, orderID, { version: 1, reason: "valid reason" }, "reward-key-0003"))
      .resolves.toMatchObject({ state: "revocation_pending" });
    expect(new Headers(fetcher.mock.calls[0][1]?.headers).get("X-Reward-Actor-ID")).toBe(actor);
  });

  it("validates exact request bodies, int64 points, versions, ids, keys and pages before fetch", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => ok(order(), 201));
    const api = createRewardsApi(fetcher, actor);
    const circular: Record<string, unknown> = { member_id: member, points: "1", reason: "circular" };
    circular.self = circular;
    const invalidGrantBodies = [
      null,
      { member_id: member, points: "1", reason: "bigint", extra: 1n },
      circular,
      { member_id: member, points: "0", reason: "zero" },
      { member_id: member, points: "01", reason: "leading zero" },
      { member_id: member, points: "9223372036854775808", reason: "overflow" },
      { member_id: member, points: 1, reason: "numeric JSON amount" },
      { member_id: member, points: "1", reason: "unknown field", extra: true },
      { member_id: member.toUpperCase(), points: "1", reason: "noncanonical member" },
      { member_id: member, points: "1", reason: "line\nbreak" },
      { member_id: member, points: "1", reason: "界".repeat(167) },
      { member_id: member, points: "1", reason: "\ud800" },
    ];
    for (const body of invalidGrantBodies) {
      await expect(api.create(brand, body as unknown as RewardGrantBody, "reward-grant-key-0010"))
        .rejects.toBeInstanceOf(RewardsApiError);
    }
    for (const body of [
      { version: 0, reason: "invalid version" },
      { version: 1.5, reason: "fractional version" },
      { version: Number.MAX_SAFE_INTEGER, reason: "cannot increment safely" },
      { version: 1, reason: "extra field", extra: true },
      { version: 1, reason: "" },
    ]) {
      await expect(api.retryRevocation(brand, orderID, body, "reward-retry-key-001"))
        .rejects.toBeInstanceOf(RewardsApiError);
    }
    for (const args of [
      ["not-a-brand", 20, 0], [brand, 0, 0], [brand, 101, 0], [brand, 20, -1], [brand, 20, 1_000_001], [brand, 1.5, 0],
    ] as const) {
      await expect(api.list(args[0], args[1], args[2])).rejects.toMatchObject({ code: "INVALID_INPUT" });
    }
    await expect(api.actions(brand, "not-an-order-id")).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.read(brand.toUpperCase(), orderID)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("snapshots each write body once for both transport and its receipt while callers mutate", async () => {
    const sentBodies: string[] = [];
    let resolveResponse!: (response: Response) => void;
    const fetcher = vi.fn<typeof fetch>(async (_input, init) => {
      sentBodies.push(String(init?.body));
      return await new Promise<Response>((resolve) => { resolveResponse = resolve; });
    });
    const api = createRewardsApi(fetcher, actor);

    const grantTarget = { member_id: member, points: "25", reason: "original grant" };
    const proxiedGrant = new Proxy(grantTarget, { get: (target, key, receiver) => Reflect.get(target, key, receiver) });
    const grantPromise = api.create(brand, proxiedGrant, "reward-grant-snapshot-01");
    grantTarget.member_id = otherBrand;
    grantTarget.points = "99";
    grantTarget.reason = "mutated grant";
    resolveResponse(ok(order({ member_id: member, points: "25", reason: "original grant" }), 201));
    await expect(grantPromise).resolves.toMatchObject({ member_id: member, points: "25", reason: "original grant" });

    const revokeBody = { version: 1, reason: "original revoke" };
    const revokePromise = api.revoke(brand, orderID, revokeBody, "reward-revoke-snapshot-01");
    revokeBody.version = 8;
    revokeBody.reason = "mutated revoke";
    resolveResponse(ok(pending({ version: 2 })));
    await expect(revokePromise).resolves.toMatchObject({ version: 2, state: "revocation_pending" });

    const retryBody = { version: 2, reason: "original retry" };
    const retryPromise = api.retryRevocation(brand, orderID, retryBody, "reward-retry-snapshot-01");
    retryBody.version = 9;
    retryBody.reason = "mutated retry";
    resolveResponse(ok(revoked({ version: 3 })));
    await expect(retryPromise).resolves.toMatchObject({ version: 3, state: "revoked" });

    expect(sentBodies).toEqual([
      JSON.stringify({ member_id: member, points: "25", reason: "original grant" }),
      JSON.stringify({ version: 1, reason: "original revoke" }),
      JSON.stringify({ version: 2, reason: "original retry" }),
    ]);
  });

  it("validates all order null keys and state, version, date and exact integer invariants", async () => {
    const validStates = [
      order(), pending(), revoked(),
    ];
    for (const value of validStates) {
      await expect(createRewardsApi(async () => ok(value)).read(brand, orderID)).resolves.toMatchObject({ state: value.state });
    }
    const malformed = [
      { ...order(), extra: "unknown" },
      (() => { const { revoked_at: _omitted, ...value } = order(); return value; })(),
      order({ points: "092" }), order({ points: "9223372036854775808" }), order({ point_policy_version: "0" }),
      order({ version: Number.MAX_SAFE_INTEGER + 1 }), order({ brand_id: otherBrand }),
      order({ state: "revocation_pending", version: 2, last_error_code: null }),
      pending({ last_error_code: "SOME_OTHER_ERROR" }), pending({ revoke_ledger_entry_id: revokeLedger }),
      revoked({ revoke_ledger_entry_id: null }), revoked({ revoked_at: null }), revoked({ last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" }),
      order({ state: "revoked", version: 1, revoke_ledger_entry_id: revokeLedger, revoked_at: now }),
      order({ created_at: "2026-02-30T10:00:00Z" }), order({ updated_at: "2026-10-07T10:00:00Z" }),
      order({ created_at: "2026-10-08T10:00:00.123457Z", updated_at: "2026-10-08T10:00:00.123456Z" }),
      order({ created_by: "NOT-A-UUID" }), order({ reason: "bad\u0085control" }), order({ reason: "界".repeat(167) }),
    ];
    for (const value of malformed) {
      await expect(createRewardsApi(async () => ok(value)).read(brand, orderID))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("strictly validates action state transitions, nulls, and exact closed DTO keys", async () => {
    const valid = [
      action(),
      action({ id: "0199a000-0000-7000-8000-000000000010", version: 2, operation: "revoke", state_before: "granted", state_after: "revocation_pending", ledger_entry_id: null }),
      action({ id: "0199a000-0000-7000-8000-000000000011", version: 3, operation: "retry", state_before: "revocation_pending", state_after: "revoked", ledger_entry_id: revokeLedger }),
    ];
    const api = createRewardsApi(async () => ok(actionPage(valid.slice().reverse(), { total_count: "3" })));
    await expect(api.actions(brand, orderID)).resolves.toMatchObject({ items: [{ version: 3 }, { version: 2 }, { version: 1 }] });

    const malformed = [
      { ...action(), extra: 1 },
      (() => { const { state_before: _omitted, ...value } = action(); return value; })(),
      action({ operation: "revoke", state_before: null, state_after: "revoked", version: 2 }),
      action({ operation: "grant", version: 2, state_before: null }),
      action({ operation: "revoke", version: 2, state_before: "granted", state_after: "revoked", ledger_entry_id: null }),
      action({ operation: "retry", version: 2, state_before: "granted", state_after: "revoked", ledger_entry_id: revokeLedger }),
      action({ operation: "retry", version: 2, state_before: "revocation_pending", state_after: "revocation_pending", ledger_entry_id: revokeLedger }),
      action({ state_after: "revoked", ledger_entry_id: "bad-id" }),
    ];
    for (const item of malformed) {
      await expect(createRewardsApi(async () => ok(actionPage([item as RewardAction]))).actions(brand, orderID))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    const repeatedActionID = action({ version: 2, operation: "revoke", state_before: "granted", state_after: "revoked", ledger_entry_id: revokeLedger });
    const duplicated = { ...repeatedActionID, version: 1, operation: "grant" as const, state_before: null, state_after: "granted" as const, ledger_entry_id: grantLedger };
    await expect(createRewardsApi(async () => ok(actionPage([repeatedActionID, duplicated], { total_count: "2" }))).actions(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("checks write receipts against actor, brand, target, submitted amount and exact next version", async () => {
    const grantBody = { member_id: member, points: "45", reason: "approved grant" };
    for (const receipt of [
      order({ created_by: otherBrand }), order({ brand_id: otherBrand }), order({ member_id: otherBrand }),
      order({ points: "44" }), order({ state: "revoked", version: 1, revoke_ledger_entry_id: revokeLedger, revoked_at: now }),
    ]) {
      await expect(createRewardsApi(async () => ok(receipt, 201), actor).create(brand, grantBody, "reward-grant-key-0020"))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(order({ id: otherBrand }))).read(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createRewardsApi(async () => ok(actionPage([action()], { order_id: otherBrand }))).actions(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    for (const receipt of [
      pending({ id: otherBrand }), pending({ brand_id: otherBrand }), pending({ version: 3 }),
      revoked({ version: 3 }),
    ]) {
      await expect(createRewardsApi(async () => ok(receipt), actor).revoke(brand, orderID, { version: 1, reason: "request reversal" }, "reward-revoke-key-02"))
        .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(order(), 200), actor).create(brand, grantBody, "reward-grant-key-0021"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("requires page response scope and query echo, sorted unique items, and exact decimal counts", async () => {
    const newerWithLowerID = order({ id: "0199a000-0000-7000-8000-000000000004", created_at: "2026-10-08T10:00:00.123457Z", updated_at: "2026-10-08T10:00:00.123457Z" });
    const olderWithHigherID = order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-08T10:00:00.123456Z", updated_at: "2026-10-08T10:00:00.123456Z" });
    const valid = orderPage([newerWithLowerID, olderWithHigherID]);
    await expect(createRewardsApi(async () => ok(valid)).list(brand)).resolves.toMatchObject({ items: [{ id: newerWithLowerID.id }, { id: olderWithHigherID.id }] });
    const invalidPages = [
      orderPage([order()], { brand_id: otherBrand }),
      orderPage([order()], { limit: 10 }),
      orderPage([order()], { offset: 1 }),
      orderPage([order(), order()], { total_count: "2" }),
      orderPage([order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-07T10:00:00Z" }), order()]),
      orderPage([
        order({ id: "0199a000-0000-7000-8000-000000000004", created_at: "2026-10-08T10:00:00.123456Z" }),
        order({ id: "0199a000-0000-7000-8000-000000000010", created_at: "2026-10-08T12:00:00.123456+02:00" }),
      ]),
      orderPage([order()], { total_count: "0" }),
      orderPage([], { total_count: "9223372036854775808" }),
    ];
    for (const page of invalidPages) {
      await expect(createRewardsApi(async () => ok(page)).list(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(createRewardsApi(async () => ok(orderPage([], { total_count: "0", offset: 200 }))).list(brand, 20, 200))
      .resolves.toMatchObject({ items: [], offset: 200 });
    await expect(createRewardsApi(async () => ok(actionPage([], { order_id: otherBrand }))).actions(brand, orderID))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(createRewardsApi(async () => ok(actionPage([], { total_count: "0", offset: 200 }))).actions(brand, orderID, 20, 200))
      .resolves.toMatchObject({ items: [], offset: 200 });
  });

  it("preserves only safe REWARD_ and AUTH_ server codes and rejects malformed envelopes", async () => {
    await expect(createRewardsApi(async () => fail("REWARD_VERSION_CONFLICT")).read(brand, orderID))
      .rejects.toMatchObject({ code: "REWARD_VERSION_CONFLICT", status: 409 });
    await expect(createRewardsApi(async () => fail("AUTH_SESSION_REVOKED", 401)).read(brand, orderID))
      .rejects.toMatchObject({ code: "AUTH_SESSION_REVOKED", status: 401 });
    await expect(createRewardsApi(async () => fail("AUTH_ACTOR_CONTEXT_CHANGED", 401)).read(brand, orderID))
      .rejects.toMatchObject({ code: "AUTH_ACTOR_CONTEXT_CHANGED", status: 401 });
    for (const malformed401 of [
      new Response("not json", { status: 401 }),
      new Response(JSON.stringify({ success: true, data: order(), request_id: requestID }), { status: 401 }),
      new Response(JSON.stringify({ success: false, error: { code: "bad code", message: "x" }, request_id: requestID }), { status: 401 }),
    ]) {
      await expect(createRewardsApi(async () => malformed401).read(brand, orderID))
        .rejects.toMatchObject({ status: 401, code: "AUTH_UNAUTHENTICATED" });
    }
    await expect(createRewardsApi(async () => fail("internal.stack.trace", 500)).read(brand, orderID))
      .rejects.toMatchObject({ code: "API_ERROR", status: 500 });
    await expect(createRewardsApi(async () => new Response(JSON.stringify({ success: true, data: order(), request_id: requestID, debug: "extra" })))
      .read(brand, orderID)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createRewardsApi(async () => new Response("not json")).read(brand, orderID))
      .rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
    await expect(createRewardsApi(async () => { throw new Error("secret network detail"); }).read(brand, orderID))
      .rejects.toMatchObject({ code: "NETWORK_ERROR", status: 0 });
  });
});
