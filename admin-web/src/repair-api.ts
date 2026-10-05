import { AdminApiError } from "./admin-api";
import type { SourceBuckets } from "./finance-api";
export interface RepairPreview {
  account_id: string;
  member_id: string;
  version: number;
  ledger_version: number;
  actual: Partial<Record<string, Partial<Record<string, string>>>>;
  expected: SourceBuckets;
  repairable: boolean;
  consistent: boolean;
  issues: string[];
  token: string;
}
export interface RepairHistory {
  id: string;
  version: number;
  before_snapshot: unknown;
  after_snapshot: unknown;
  reason: string;
  actor_id: string;
  request_id: string;
  created_at: string;
}
export function createRepairApi(fetcher: typeof fetch = fetch) {
  async function request<T>(
    brand: string,
    member: string,
    suffix: string,
    body?: unknown,
    key?: string,
  ): Promise<T> {
    const headers = new Headers({
      "X-Brand-ID": brand,
      Accept: "application/json",
    });
    if (body !== undefined) headers.set("Content-Type", "application/json");
    if (key) headers.set("Idempotency-Key", key);
    const r = await fetcher(
      `/api/v1/admin/wallets/${encodeURIComponent(member)}/${suffix}`,
      {
        method: body === undefined ? "GET" : "POST",
        credentials: "same-origin",
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      },
    );
    const out = await r.json();
    if (!r.ok || !out.success)
      throw new AdminApiError(
        out.error?.message || "差错处理暂不可用",
        r.status,
        out.error?.code,
      );
    return out.data;
  }
  return {
    preview: (brand: string, member: string) =>
      request<RepairPreview>(brand, member, "repair-preview"),
    history: (brand: string, member: string) =>
      request<{ items: RepairHistory[] }>(brand, member, "repairs"),
    repair: (
      brand: string,
      member: string,
      body: { version: number; token: string; reason: string },
      key: string,
    ) =>
      request<{ id: string; audit_log_id: string }>(
        brand,
        member,
        "repair",
        body,
        key,
      ),
  };
}
