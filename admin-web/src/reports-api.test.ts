import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createReportsApi,
  reportsPermissions,
  type BettingTotals,
  type LedgerTotals,
  type ReportQuery,
} from "./reports-api";

const brand = "018f47a1-7b2c-7abc-8def-0123456789ab";
const game = "018f47a1-7b2c-7abc-8def-0123456789ac";
const member = "018f47a1-7b2c-7abc-8def-0123456789ad";
const foreignBrand = "018f47a1-7b2c-7abc-8def-0123456789ae";
const from = "2026-01-01T00:00:00Z";
const to = "2026-04-04T00:00:00Z"; // exactly 93 days
const query: ReportQuery = { from, to, group_by: "day" };
const bettingTotals = (): BettingTotals => ({
  order_count: "5", stake_points: "20", placed_count: "1", won_count: "1",
  lost_count: "1", abnormal_count: "1", cancelled_count: "1", refund_points: "4",
  settled_stake_points: "10", unfinalized_stake_points: "4", abnormal_stake_points: "2",
  current_prize_points: "5", correction_open_count: "0",
});
const ledgerTotals = (overrides: Partial<LedgerTotals> = {}): LedgerTotals => ({
  entry_count: "2", net_points: "0", recharge_points: "0", prize_credit_points: "12",
  prize_reversal_points: "12", refund_points: "0", ...overrides,
});
function payload(
  kind: "betting" | "ledger",
  overrides: Record<string, unknown> = {},
) {
  const body: Record<string, unknown> = {
    brand_id: brand,
    snapshot_at: "2026-04-04T12:00:00Z",
    timezone: "Asia/Singapore",
    query: { from, to, group_by: "day", limit: 20, offset: 0, game_id: null, member_id: null },
    summary: kind === "betting" ? bettingTotals() : ledgerTotals(),
    items: [{ key: "2026-04-03", label: "2026-04-03", totals: kind === "betting" ? bettingTotals() : ledgerTotals() }],
    total_groups: "1",
    ...(kind === "ledger" ? { balances: { account_count: "1", available_points: "30", frozen_points: "4", withdrawal_points: "6", total_points: "40" } } : {}),
    ...overrides,
  };
  return body;
}
const response = (data: unknown, status = 200) => new Response(
  JSON.stringify({ success: true, data }), { status, headers: { "Content-Type": "application/json" } },
);
const fetcher = (data: unknown) => vi.fn<typeof fetch>().mockResolvedValue(response(data));

describe("reports permissions", () => {
  const account: AdminAccount = {
    id: brand,
    super_admin: false,
    brand_ids: [brand],
    permissions: [],
    permissions_by_brand: { [brand]: ["report_betting.view.brand"] },
  };

  it("keeps capabilities separate and uses exact brand and platform grants without implicit super privileges", () => {
    expect(reportsPermissions(account, brand)).toEqual({ betting: true, ledger: false });
    const platform = { ...account, super_admin: true, platform_permissions: ["report_ledger.view.platform"] };
    expect(reportsPermissions(platform, brand)).toEqual({ betting: true, ledger: true });
    expect(reportsPermissions({...platform,brand_ids:[]},foreignBrand)).toEqual({betting:false,ledger:true});
    expect(reportsPermissions({ ...platform, super_admin: false }, brand).ledger).toBe(true);
    expect(reportsPermissions(account, foreignBrand)).toEqual({ betting: false, ledger: false });
  });
  it("rejects flattened permissions and invalid scopes",()=>{
    expect(reportsPermissions({...account,permissions:["report_ledger.view.brand"],permissions_by_brand:undefined},brand)).toEqual({betting:false,ledger:false});
    expect(reportsPermissions({...account,super_admin:true,permissions_by_brand:{},platform_permissions:[]},brand)).toEqual({betting:false,ledger:false});
    expect(reportsPermissions({...account,id:"invalid",platform_permissions:["report_ledger.view.platform"]},brand)).toEqual({betting:false,ledger:false});
  });
});

describe("reports API", () => {
  it("matches equivalent UTC echoes exactly without losing nanosecond differences",async()=>{
    const data=payload("ledger");
    const formQuery={...query,from:"2026-01-01T08:00:00.000+08:00",to:"2026-04-04T00:00:00.000Z"};
    await expect(createReportsApi(fetcher(data)).ledger(brand,formQuery)).resolves.toMatchObject({query:{from,to}});
    const changed=payload("ledger",{query:{from:"2026-01-01T00:00:00.000000001Z",to,group_by:"day",limit:20,offset:0,game_id:null,member_id:null}});
    await expect(createReportsApi(fetcher(changed)).ledger(brand,formQuery)).rejects.toMatchObject({code:"INVALID_RESPONSE"});
  });
  it("sends a cookie-authenticated, brand-scoped GET with defaults and returns arbitrary precision values unchanged", async () => {
    const huge = "922337203685477580812345678901234567890";
    const hugeTotals={...bettingTotals(),order_count:huge,stake_points:huge,placed_count:huge,won_count:"0",lost_count:"0",abnormal_count:"0",cancelled_count:"0",refund_points:"0",abnormal_stake_points:"0",settled_stake_points:"0",unfinalized_stake_points:huge,current_prize_points:"0"};
    const data = payload("betting", {
      summary: hugeTotals,
      items: [{ key: "2026-04-03", label: "2026-04-03", totals: hugeTotals }],
    });
    const call = fetcher(data);
    const result = await createReportsApi(call).betting(brand, query);
    expect(result.summary.order_count).toBe(huge);
    expect(call).toHaveBeenCalledOnce();
    const [url, init] = call.mock.calls[0]!;
    expect(String(url)).toContain("/api/v1/admin/reports/betting?");
    const params = new URL(String(url), "http://localhost").searchParams;
    expect(Object.fromEntries(params)).toEqual({ from, to, group_by: "day", limit: "20", offset: "0" });
    expect(init).toMatchObject({ method: "GET", credentials: "same-origin" });
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Accept")).toBe("application/json");
  });

  it("preserves final betting projection separately from posted ledger history", async () => {
    const betting = payload("betting", {
      summary: { ...bettingTotals(), current_prize_points: "0" },
      items: [{ key: "2026-04-03", label: "2026-04-03", totals: { ...bettingTotals(), current_prize_points: "0" } }],
    });
    const ledger = payload("ledger", {
      query: { from, to, group_by: "entry_type", limit: 20, offset: 0, game_id: null, member_id: null },
      summary: ledgerTotals({ entry_count: "3", net_points: "0", prize_credit_points: "9", prize_reversal_points: "9" }),
      items: [
        { key: "prize", label: "prize", totals: ledgerTotals({ entry_count: "2", net_points: "9", prize_credit_points: "9", prize_reversal_points: "0" }) },
        { key: "prize_reversal", label: "prize_reversal", totals: ledgerTotals({ entry_count: "1", net_points: "-9", prize_credit_points: "0", prize_reversal_points: "9" }) },
      ],
      total_groups: "2",
    });
    const bettingResult = await createReportsApi(fetcher(betting)).betting(brand, query);
    const ledgerResult = await createReportsApi(fetcher(ledger)).ledger(brand, { ...query, group_by: "entry_type" });
    expect(bettingResult.summary.current_prize_points).toBe("0");
    expect(ledgerResult.summary.prize_credit_points).toBe("9");
    expect(ledgerResult.summary.prize_reversal_points).toBe("9");
    expect(ledgerResult.items.map((item) => item.key)).toEqual(["prize", "prize_reversal"]);
    expect(ledgerResult.balances.total_points).toBe("40");
  });

  it("accepts operation keys up to the backend's exact 200-character pattern and rejects unsafe keys", async () => {
    const operation = `custom:${"x".repeat(193)}`;
    const data = payload("ledger", {
      query: { from, to, group_by: "entry_type", limit: 20, offset: 0, game_id: null, member_id: null },
      items: [{ key: operation, label: operation, totals: ledgerTotals() }],
    });
    await expect(createReportsApi(fetcher(data)).ledger(brand, { ...query, group_by: "entry_type" })).resolves.toMatchObject({ items: [{ key: operation }] });
    const unsafe = structuredClone(data);
    (unsafe.items as { key: string; label: string }[])[0]!.key = "contains space";
    (unsafe.items as { key: string; label: string }[])[0]!.label = "contains space";
    await expect(createReportsApi(fetcher(unsafe)).ledger(brand, { ...query, group_by: "entry_type" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("accepts a 93-day half-open request window and rejects invalid, reversed, or longer windows locally", async () => {
    const call = fetcher(payload("betting"));
    await createReportsApi(call).betting(brand, query);
    for (const invalid of [
      { ...query, from: "2026-02-30T00:00:00Z" },
      { ...query, from: to, to: from },
      { ...query, to: "2026-04-04T00:00:00.001Z" },
      { ...query, from: "2026-01-01" },
    ]) {
      await expect(createReportsApi(call).betting(brand, invalid)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    }
    expect(call).toHaveBeenCalledOnce();
  });

  it("rejects unsupported filters, groups, invalid UUIDs, and out-of-range pagination before fetching", async () => {
    const call = fetcher(payload("ledger"));
    const api = createReportsApi(call);
    await expect(api.ledger(brand, { ...query, game_id: game })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.betting(brand, { ...query, group_by: "entry_type" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.ledger(brand, { ...query, group_by: "game" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.betting(brand, { ...query, member_id: "not-a-uuid" })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.betting(brand, { ...query, limit: 101 })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    await expect(api.ledger(brand, { ...query, offset: 1_000_001 })).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(call).not.toHaveBeenCalled();
  });

  it("rejects foreign brands, mismatched query echoes, unsafe labels, duplicate keys, and malformed exact totals", async () => {
    const foreign = payload("betting", { brand_id: foreignBrand });
    await expect(createReportsApi(fetcher(foreign)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const mismatched = payload("betting", { query: { from, to, group_by: "day", limit: 20, offset: 0, game_id: game, member_id: null } });
    await expect(createReportsApi(fetcher(mismatched)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const namedMember = payload("betting", { query: { from, to, group_by: "member", limit: 20, offset: 0, game_id: null, member_id: null }, items: [{ key: member, label: "Private Member Name", totals: bettingTotals() }] });
    await expect(createReportsApi(fetcher(namedMember)).betting(brand, { ...query, group_by: "member" })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const duplicate = payload("betting", { items: [
      { key: "2026-04-03", label: "2026-04-03", totals: bettingTotals() },
      { key: "2026-04-03", label: "2026-04-03", totals: bettingTotals() },
    ], total_groups: "2" });
    await expect(createReportsApi(fetcher(duplicate)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    for (const malformed of ["01", "-1", 1, "1e3", "9".repeat(129)]) {
      const badTotals = payload("betting", { summary: { ...bettingTotals(), stake_points: malformed } });
      await expect(createReportsApi(fetcher(badTotals)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
  });

  it("checks totals partitions and rejects an unreconciled ledger balance envelope", async () => {
    const badPartition = payload("betting", { summary: { ...bettingTotals(), settled_stake_points: "11" } });
    await expect(createReportsApi(fetcher(badPartition)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badStatus = payload("betting", { items: [{ key: "2026-04-03", label: "2026-04-03", totals: { ...bettingTotals(), order_count: "6" } }] });
    await expect(createReportsApi(fetcher(badStatus)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const badBalances = payload("ledger", { balances: { account_count: "1", available_points: "30", frozen_points: "4", withdrawal_points: "6", total_points: "41" } });
    await expect(createReportsApi(fetcher(badBalances)).ledger(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("preserves standard API error status, code, and message", async () => {
    const call = vi.fn<typeof fetch>().mockResolvedValue(new Response(
      JSON.stringify({ success: false, error: { code: "FORBIDDEN", message: "denied" } }), { status: 403 },
    ));
    const result = createReportsApi(call).ledger(brand, query);
    await expect(result).rejects.toBeInstanceOf(AdminApiError);
    await expect(result).rejects.toMatchObject({ status: 403, code: "FORBIDDEN", message: "denied" });
  });
});
