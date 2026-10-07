import { describe, expect, it, vi } from "vitest";
import { createRechargeApi, isMemberRecharge, isMemberRechargePage, rechargeBasePath } from "./recharge-api";

const brand = "11111111-1111-4111-8111-111111111111";
const member = "22222222-2222-4222-8222-222222222222";
const otherMember = "33333333-3333-4333-8333-333333333333";
const id = "44444444-4444-4444-8444-444444444444";
const otherId = "66666666-6666-4666-8666-666666666666";
const ledgerId = "55555555-5555-4555-8555-555555555555";
const at = "2026-10-07T12:00:00Z";
const pending = { id, brand_id: brand, member_id: member, points: "9007199254740993", state: "pending", version: "1", created_at: at, confirmed_at: null, ledger_entry_id: null };
const confirmed = { ...pending, state: "confirmed", version: "2", confirmed_at: at, ledger_entry_id: ledgerId };
const page = { brand_id: brand, member_id: member, snapshot_at: at, state: null, items: [pending], limit: 20, offset: 0, total_count: "9007199254740993" };
const response = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status });
const scope = { brand_id: brand, member_id: member };

describe("member recharge read API", () => {
  it("issues same-origin GET requests with only the supported query parameters", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(page));
    const controller = new AbortController();
    const result = await createRechargeApi({ brandCode: "north star", fetcher }).list(scope, {}, controller.signal);
    expect(result).toEqual(page);
    const [url, init] = fetcher.mock.calls[0]!;
    expect(url).toBe("/api/v1/b/north%20star/recharges?limit=20&offset=0");
    expect(rechargeBasePath("north star")).toBe("/api/v1/b/north%20star");
    expect(init).toMatchObject({ method: "GET", credentials: "same-origin", signal: controller.signal });
    expect(new Headers(init?.headers).get("Accept")).toBe("application/json");
    expect(init?.body).toBeUndefined();
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
    const parsed = new URL(String(url), "https://local.test");
    expect([...parsed.searchParams.keys()]).toEqual(["limit", "offset"]);
  });

  it("sends the selected filter and validates exact response scope, target and paging", async () => {
    const filtered = { ...page, state: "confirmed", items: [confirmed], limit: 10, offset: 20 };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(filtered));
    const result = await createRechargeApi({ fetcher }).list(scope, { limit: 10, offset: 20, state: "confirmed" });
    expect(result).toEqual(filtered);
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/v1/recharges?limit=10&offset=20&state=confirmed");
    for (const bad of [
      { ...filtered, member_id: otherMember },
      { ...filtered, items: [{ ...confirmed, member_id: otherMember }] },
      { ...filtered, state: null },
      { ...filtered, limit: 20 },
      { ...filtered, total_count: "01" },
      { ...filtered, internal_reason: "sensitive" },
    ]) {
      const api = createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(bad)) });
      await expect(api.list(scope, { limit: 10, offset: 20, state: "confirmed" })).rejects.toThrow();
    }
  });

  it("validates both state branches, closed DTOs, UUIDs, dates and int64 strings", () => {
    expect(isMemberRecharge(pending)).toBe(true);
    expect(isMemberRecharge(confirmed)).toBe(true);
    expect(isMemberRecharge({ ...pending, created_at: "0001-01-01T00:00:00Z" })).toBe(true);
    expect(isMemberRechargePage(page)).toBe(true);
    for (const bad of [
      { ...pending, extra: "private" },
      { ...pending, account_id: "66666666-6666-4666-8666-666666666666" },
      { ...pending, state: "confirmed" },
      { ...pending, points: "01" },
      { ...pending, points: "9223372036854775808" },
      { ...pending, version: "0" },
      { ...pending, id: "not-a-uuid" },
      { ...pending, created_at: "2026-02-30T12:00:00Z" },
      { ...pending, created_at: "0000-01-01T00:00:00Z" },
      { ...pending, created_at: "2026-10-07T12:00:00+00:00" },
      { ...pending, state: "cancelled", ledger_entry_id: ledgerId },
    ]) expect(isMemberRecharge(bad)).toBe(false);
    expect(isMemberRecharge({ ...confirmed, confirmed_at: null })).toBe(false);
  });

  it("accepts an empty page beyond total_count but rejects nonempty overrun, duplicate IDs and invalid ordering", async () => {
    const emptyBeyondTotal = { ...page, items: [], offset: 20, total_count: "0" };
    const emptyAPI = createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(emptyBeyondTotal)) });
    await expect(emptyAPI.list(scope, { offset: 20 })).resolves.toEqual(emptyBeyondTotal);

    const nanos = (fraction: string) => `2026-10-07T12:00:00.${fraction}Z`;
    const newest = { ...pending, id: "77777777-7777-4777-8777-777777777777", created_at: nanos("000000002") };
    const tiedHigherID = { ...pending, id: otherId, created_at: nanos("000000001") };
    const tiedLowerID = { ...pending, created_at: nanos("000000001") };
    const ordered = { ...page, items: [newest, tiedHigherID, tiedLowerID], total_count: "3" };
    await expect(createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(ordered)) }).list(scope)).resolves.toEqual(ordered);

    const invalidPages = [
      { ...emptyBeyondTotal, items: [pending] },
      { ...page, items: [pending, pending], total_count: "2" },
      { ...page, items: [tiedLowerID, tiedHigherID, newest], total_count: "3" },
    ];
    for (const invalidPage of invalidPages) {
      await expect(createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(invalidPage)) }).list(scope, { offset: invalidPage.offset })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
  });

  it("reads one exact record through GET only and rejects cross-record or cross-member responses", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response(confirmed));
    const result = await createRechargeApi({ fetcher }).get(scope, id);
    expect(result).toEqual(confirmed);
    expect(fetcher.mock.calls[0]?.[0]).toBe(`/api/v1/recharges/${id}`);
    expect(fetcher.mock.calls[0]?.[1]).toMatchObject({ method: "GET", credentials: "same-origin" });
    expect(fetcher.mock.calls[0]?.[1]?.body).toBeUndefined();
    for (const wrong of [{ ...confirmed, id: otherMember }, { ...confirmed, member_id: otherMember }, { ...confirmed, proof_reference: "sensitive" }]) {
      await expect(createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(wrong)) }).get(scope, id)).rejects.toThrow();
    }
  });

  it("rejects invalid caller input before any request", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createRechargeApi({ fetcher });
    await expect(api.list(scope, { limit: 0 })).rejects.toThrow(TypeError);
    await expect(api.list(scope, { offset: 1_000_001 })).rejects.toThrow(TypeError);
    await expect(api.get(scope, "bad-id")).rejects.toThrow(TypeError);
    await expect(api.list({ ...scope, member_id: "invalid" })).rejects.toThrow(TypeError);
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("requires readable successful envelopes and preserves HTTP status", async () => {
    const api = createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response("gateway", { status: 503 })) });
    await expect(api.list(scope)).rejects.toMatchObject({ status: 503 });
    const invalid = createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({ success: false, data: page }))) });
    await expect(invalid.list(scope)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const unexpectedSuccessStatus = createRechargeApi({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(response(page, 201)) });
    await expect(unexpectedSuccessStatus.list(scope)).rejects.toMatchObject({ code: "INVALID_RESPONSE", status: 502 });
  });
});
