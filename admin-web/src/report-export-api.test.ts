import { describe, expect, it, vi } from "vitest";
import { type AdminAccount } from "./admin-api";
import { createReportExportApi, reportExportPermissions, type ExportKind, type ExportQuery } from "./report-export-api";

const brand = "018f47a1-7b2c-7abc-8def-0123456789ab";
const foreign = "018f47a1-7b2c-7abc-8def-0123456789ac";
const query: ExportQuery = { from: "2026-01-01T00:00:00Z", to: "2026-01-02T00:00:00Z", group_by: "day" };
const common = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "key", "label"];
const betting = ["order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count", "cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points", "abnormal_stake_points", "current_prize_points", "correction_open_count"];
const ledger = ["entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points", "account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"];
const snapshot = "2026-01-02T03:04:05.123456Z";

function csvCell(value: string): string { return /[",\n]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value; }
function responseFor(kind: ExportKind, options: { label?: string; foreignBrand?: boolean; hash?: string; contentLength?: string; badColumns?: boolean; wrongGroupTotal?: boolean; groupCount?: number; groupBy?: "day" | "game" | "entry_type"; keys?: string[]; omitBalances?: boolean; badBalance?: boolean } = {}) {
  const groupCount = options.groupCount ?? options.keys?.length ?? 1;
  const groupBy = options.groupBy ?? (groupCount > 1 ? "game" : "day");
  const fields = kind === "betting" ? betting : ledger;
  const columns = [...common, ...fields];
  if (options.badColumns) columns[0] = "unexpected";
  const metadata = ["018f47a1-7b2c-7abc-8def-0123456789ab", snapshot, "Asia/Singapore", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", groupBy, "", "", "", ""];
  const huge = "9".repeat(160);
  const totals: string[] = fields.slice(0, kind === "ledger" ? 6 : fields.length).map((field) => field === "entry_count" && options.keys ? String(groupCount) : field === "net_points" ? "-9" : field === "stake_points" && groupCount === 1 ? huge : "0");
  const summary = ["summary", ...metadata, ...totals, ...(kind === "ledger" ? ["", "", "", "", ""] : [])];
  const groupKey = groupBy === "game" ? "018f47a1-7b2c-7abc-8def-000000000000" : groupBy === "entry_type" ? (options.keys?.[0] ?? "-danger") : "2026-01-01";
  const label = options.label ?? "2026-01-01";
  const groupTotals = [...totals];
  if (options.wrongGroupTotal) groupTotals[0] = "1";
  const rows = [columns, summary];
  if (kind === "ledger" && !options.omitBalances) rows.push(["balances", ...metadata.slice(0, 8), "", "", ...Array(6).fill(""), "1", "2", "3", "4", options.badBalance ? "10" : "9"]);
  for (let i = 0; i < groupCount; i++) {
    const rawKey = options.keys?.[i] ?? (groupBy === "game" && groupCount > 1 ? `018f47a1-7b2c-7abc-8def-${i.toString(16).padStart(12, "0")}` : groupKey);
    const rawLabel = options.keys ? rawKey : groupCount > 1 ? `Game ${String(i).padStart(5, "0")}` : label;
    const escapeText = (value: string) => /^[\s\u0000-\u001f\u007f\uFEFF]*[=+\-@]/.test(value) ? `'${value}` : value;
    const key = escapeText(rawKey), groupLabel = escapeText(rawLabel);
    const rowTotals = options.keys && kind === "ledger" ? fields.slice(0, 6).map((field) => field === "entry_count" ? "1" : field === "net_points" ? `-${i === 0 ? Math.ceil(9 / groupCount) : Math.floor(9 / groupCount)}` : "0") : groupCount > 1 ? fields.slice(0, kind === "ledger" ? 6 : fields.length).map(() => "0") : groupTotals;
    rows.push(["group", ...metadata.slice(0, 8), key, groupLabel, ...rowTotals, ...(kind === "ledger" ? ["", "", "", "", ""] : [])]);
  }
  const text = `${rows.map((row) => row.map(csvCell).join(",")).join("\n")}\n`;
  const body = new TextEncoder().encode(`\ufeff${text}`);
  const brandId = options.foreignBrand ? foreign : brand;
  if (options.foreignBrand) {
    const lines = text.split("\n");
    const parts = lines[2]!.split(","); parts[1] = foreign; lines[2] = parts.join(",");
  }
  const filename = `lottery-${kind}-${brand}-20260102T030405Z`;
  return { body, headers: new Headers({
    "Content-Type": "text/csv; charset=utf-8",
    "Content-Disposition": `attachment; filename="${filename}.csv"`,
    "Content-Length": options.contentLength ?? String(body.length),
    "Cache-Control": "no-store",
    "X-Report-Brand-ID": brandId,
    "X-Report-Kind": kind,
    "X-Report-Snapshot-At": snapshot,
    "X-Report-Group-Count": String(groupCount),
    "X-Report-SHA256": options.hash ?? "0".repeat(64),
    "X-Report-Format-Version": "1",
    "X-Report-Audit-ID": "018f47a1-7b2c-7abc-8def-0123456789ad",
  }) };
}

async function signedResponse(kind: ExportKind, options: Parameters<typeof responseFor>[1] = {}) {
  const result = responseFor(kind, options);
  result.headers.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", result.body))].map((n) => n.toString(16).padStart(2, "0")).join(""));
  return result;
}
function fetchWith(response: Response) { return vi.fn<typeof fetch>().mockResolvedValue(response); }
async function validResponse(kind: ExportKind, options: Parameters<typeof responseFor>[1] = {}) {
  const result = await signedResponse(kind, options);
  return new Response(result.body, { status: 200, headers: result.headers });
}

describe("report export permissions", () => {
  const base: AdminAccount = { id: brand, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: [] } };
  it("requires separate mapped brand view and export grants", () => {
    expect(reportExportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_betting.view.brand", "report_betting.export.brand"] } }, brand).betting).toBe(true);
    expect(reportExportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_betting.view.brand"] }, platform_permissions: ["report_betting.export.platform"] }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, permissions: ["report_betting.view.brand", "report_betting.export.brand"], permissions_by_brand: { [brand]: [] } }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, permissions: ["report_betting.view.brand", "report_betting.export.brand"], permissions_by_brand: undefined }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_betting.export.brand"] } }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_betting.export.brand"] }, platform_permissions: ["report_betting.view.platform"] }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, permissions_by_brand: { [brand]: ["report_betting.view.brand", "report_betting.export.brand"] }, brand_ids: [] }, brand).betting).toBe(false);
    expect(reportExportPermissions({ ...base, super_admin: true }, brand)).toEqual({ betting: false, ledger: false });
    expect(reportExportPermissions({ ...base, brand_ids: [], platform_permissions: ["report_ledger.view.platform", "report_ledger.export.platform"] }, foreign).ledger).toBe(false);
  });
});

describe("report CSV export API", () => {
  it("requests only committed filters, preserves huge decimal strings, and returns verified bytes", async () => {
    const call = fetchWith(await validResponse("betting"));
    const result = await createReportExportApi(call).betting(brand, query);
    expect(result).toMatchObject({ groupCount: "1", snapshotAt: snapshot, auditLogId: "018f47a1-7b2c-7abc-8def-0123456789ad" });
    expect(new TextDecoder().decode(result.bytes)).toContain("9".repeat(160));
    const [url, init] = call.mock.calls[0]!;
    const parsed = new URL(String(url), "http://localhost");
    expect(parsed.pathname).toBe("/api/v1/admin/reports/betting/export");
    expect(Object.fromEntries(parsed.searchParams)).toEqual({ from: query.from, to: query.to, group_by: "day" });
    expect(init).toMatchObject({ method: "GET", credentials: "same-origin" });
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
  });

  it("accepts quoted formula-escaped labels and exact equivalent nanosecond filter instants", async () => {
    const formulaQuery = { ...query, from: "2026-01-01T08:00:00.000000000+08:00" };
    const response = await validResponse("betting", { groupBy: "game", label: "  =SUM(\"a\",\n\"b\")" });
    await expect(createReportExportApi(fetchWith(response)).betting(brand, { ...formulaQuery, group_by: "game" })).resolves.toMatchObject({ groupCount: "1" });
  });

  it("accepts signed ledger flow with a complete balance row and sorts sanitized operation keys by their originals", async () => {
    const ledgerQuery = { ...query, group_by: "entry_type" as const };
    const complete = await validResponse("ledger", { groupBy: "entry_type", keys: ["-a", ":a", "Aa"] });
    const exported = await createReportExportApi(fetchWith(complete)).ledger(brand, ledgerQuery);
    expect(exported.groupCount).toBe("3");
    const rows = new TextDecoder().decode(exported.bytes.subarray(3)).trimEnd().split("\n").map((row) => row.split(","));
    expect(rows[1]![12]).toBe("-9");
    expect(rows[2]![0]).toBe("balances");
    expect(rows.slice(3).map((row) => row[9])).toEqual(["'-a", ":a", "Aa"]);
    for (const options of [{ omitBalances: true, groupBy: "entry_type" as const, label: "-danger" }, { badBalance: true, groupBy: "entry_type" as const, label: "-danger" }]) {
      const invalid = await validResponse("ledger", options);
      await expect(createReportExportApi(fetchWith(invalid)).ledger(brand, ledgerQuery)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    }
    const unsupportedEntryType = await validResponse("ledger", { groupBy: "entry_type", keys: ["@a"] });
    await expect(createReportExportApi(fetchWith(unsupportedEntryType)).ledger(brand, ledgerQuery)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("hashes and validates every group in the complete 10,000-group CSV", async () => {
    const result = await signedResponse("betting", { groupCount: 10_000 });
    const response = new Response(result.body, { status: 200, headers: result.headers });
    const exported = await createReportExportApi(fetchWith(response)).betting(brand, { ...query, group_by: "game" });
    expect(exported.groupCount).toBe("10000");
    const lines = new TextDecoder().decode(exported.bytes.subarray(3)).split("\n");
    expect(lines).toHaveLength(10_003);
    expect(lines[0]).toContain("record_type");
    expect(lines[10_001]).toContain("group");
  });

  it("rejects foreign brand scope, bad columns, incorrect byte length, and hash mismatch before returning a file", async () => {
    const foreignResponse = await validResponse("betting", { foreignBrand: true });
    await expect(createReportExportApi(fetchWith(foreignResponse)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const columnsResponse = await validResponse("betting", { badColumns: true });
    await expect(createReportExportApi(fetchWith(columnsResponse)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const wrongKind = await validResponse("betting");
    wrongKind.headers.set("X-Report-Kind", "ledger");
    await expect(createReportExportApi(fetchWith(wrongKind)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const lengthResponse = await validResponse("betting", { contentLength: "999" });
    await expect(createReportExportApi(fetchWith(lengthResponse)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const hashResponse = await validResponse("betting");
    hashResponse.headers.set("X-Report-SHA256", "0".repeat(64));
    await expect(createReportExportApi(fetchWith(hashResponse)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const mismatchedTotals = await validResponse("betting", { wrongGroupTotal: true });
    await expect(createReportExportApi(fetchWith(mismatchedTotals)).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("rejects malformed UTF-8, CSV, and metadata and preserves server error codes", async () => {
    const malformed = await validResponse("betting");
    const bytes = new Uint8Array(await malformed.arrayBuffer());
    bytes[bytes.length - 3] = 0xff;
    malformed.headers.set("Content-Length", String(bytes.length));
    malformed.headers.set("X-Report-SHA256", [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))].map((n) => n.toString(16).padStart(2, "0")).join(""));
    await expect(createReportExportApi(fetchWith(new Response(bytes, { status: 200, headers: malformed.headers }))).betting(brand, query)).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
    const denied = new Response(JSON.stringify({ success: false, error: { code: "FORBIDDEN", message: "denied" } }), { status: 403 });
    await expect(createReportExportApi(fetchWith(denied)).betting(brand, query)).rejects.toMatchObject({ status: 403, code: "FORBIDDEN", message: "denied" });
    const failed = new Response("upstream HTML", { status: 502, headers: { "Content-Type": "text/html" } });
    await expect(createReportExportApi(fetchWith(failed)).betting(brand, query)).rejects.toMatchObject({ status: 502 });
    const call = vi.fn<typeof fetch>();
    await expect(createReportExportApi(call).betting(brand, { ...query, offset: 20 } as ExportQuery)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    expect(call).not.toHaveBeenCalled();
  });
});
