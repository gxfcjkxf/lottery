import { readReportDownload } from "@lottery/shared";
import { readAdminErrorResponse } from './admin-api'

import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";
import type { ReportGroupBy, ReportQuery } from "./reports-api";

const BASE = "/api/v1/admin/reports";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_WINDOW_NS = 93n * 24n * 60n * 60n * 1_000_000_000n;

export type ExportKind = "betting" | "ledger";
export type ReportExport = { filename: string; bytes: Uint8Array; groupCount: string; snapshotAt: string; auditLogId: string };
export type ExportQuery = Pick<ReportQuery, "from" | "to" | "group_by" | "game_id" | "member_id">;

function fail(message = "报表导出响应格式无效。", status = 502, code = "INVALID_RESPONSE"): never {
  throw new AdminApiError(message, status, code);
}
function badInput(message: string): never { return fail(message, 0, "INVALID_INPUT"); }

function validDateTime(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , zone, , zh, zm] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2 ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28) : ([4, 6, 9, 11].includes(month) ? 30 : 31);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days || Number(h) > 23 || Number(mi) > 59 || Number(s) > 59) return false;
  return zone === "Z" || (Number(zh) <= 14 && Number(zm) <= 59 && !(Number(zh) === 14 && Number(zm) !== 0));
}
function epochNs(value: string): bigint {
  const fraction = /\.(\d{1,9})(?=Z|[+-]\d{2}:\d{2}$)/.exec(value)?.[1] ?? "";
  const milliseconds = Date.parse(value.replace(/\.\d+(?=Z|[+-]\d{2}:\d{2}$)/, ""));
  if (!Number.isFinite(milliseconds)) badInput("报表日期无效。");
  return BigInt(milliseconds) * 1_000_000n + BigInt(fraction.padEnd(9, "0") || "0");
}
function validateQuery(kind: ExportKind, query: ExportQuery): ExportQuery {
  if (!query || !validDateTime(query.from) || !validDateTime(query.to)) badInput("from 和 to 必须是有效 RFC3339 日期时间。");
  if (Object.keys(query).some((key) => !["from", "to", "group_by", "game_id", "member_id"].includes(key))) badInput("导出查询包含不支持的参数。");
  const start = epochNs(query.from), end = epochNs(query.to);
  if (end <= start || end - start > MAX_WINDOW_NS) badInput("报告时间范围必须大于零且不超过 93 天。");
  const groups: readonly ReportGroupBy[] = kind === "betting" ? ["day", "game", "member"] : ["day", "entry_type"];
  if (!groups.includes(query.group_by)) badInput("group_by 不适用于此报告。");
  if (query.game_id !== undefined && (kind !== "betting" || !UUID_RE.test(query.game_id))) badInput("game_id 不适用于此报告或 UUID 格式无效。");
  if (query.member_id !== undefined && !UUID_RE.test(query.member_id)) badInput("member_id UUID 格式无效。");
  return { from: query.from, to: query.to, group_by: query.group_by, ...(query.game_id ? { game_id: query.game_id } : {}), ...(query.member_id ? { member_id: query.member_id } : {}) };
}
export function reportExportPermissions(account: AdminAccount, brandId: string): { betting: boolean; ledger: boolean } {
  const scoped = UUID_RE.test(account.id) && UUID_RE.test(brandId);
  const brandGrants = brandPermissionSet(account, brandId.toLowerCase());
  const allowed = (kind: ExportKind) => {
    const base = `report_${kind}`;
    return scoped && brandGrants.has(`${base}.view.brand`) && brandGrants.has(`${base}.export.brand`);
  };
  return { betting: allowed("betting"), ledger: allowed("ledger") };
}

export function createReportExportApi(fetchImpl: typeof fetch = fetch) {
  async function request(kind: ExportKind, brandId: string, input: ExportQuery): Promise<ReportExport> {
    if (!UUID_RE.test(brandId)) badInput("brand 必须是 UUID。");
    const query = validateQuery(kind, input);
    const params = new URLSearchParams({ from: query.from, to: query.to, group_by: query.group_by });
    if (query.game_id) params.set("game_id", query.game_id);
    if (query.member_id) params.set("member_id", query.member_id);
    const response = await fetchImpl(`${BASE}/${kind}/export?${params}`, { method: "GET", credentials: "same-origin", headers: { Accept: "text/csv", "X-Brand-ID": brandId } });
    if (!response.ok) throw await readAdminErrorResponse(response);
    return readReportDownload(response, brandId, kind, () => fail());
  }
  return {
    betting(brandId: string, query: ExportQuery) { return request("betting", brandId, query); },
    ledger(brandId: string, query: ExportQuery) { return request("ledger", brandId, query); },
  };
}
