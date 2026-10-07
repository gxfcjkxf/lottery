import { AdminApiError } from "./admin-api";
import type { SourceBuckets } from "./finance-api";
import { normalizeSourceBuckets } from "@lottery/shared";
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
function normalizeHistoricalSnapshot(value: unknown): unknown {
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  const snapshot = value as Record<string, unknown>;
  const normalized = normalizeSourceBuckets(snapshot.buckets);
  if (normalized) return { ...snapshot, buckets: normalized };
  const direct = normalizeSourceBuckets(value);
  return direct ?? value;
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
    async history(brand: string, member: string) {
      const result = await request<{ items: RepairHistory[] }>(brand, member, "repairs");
      if (!result || !Array.isArray(result.items)) return result;
      return {
        ...result,
        items: result.items.map((item) => ({
          ...item,
          before_snapshot: normalizeHistoricalSnapshot(item.before_snapshot),
          after_snapshot: normalizeHistoricalSnapshot(item.after_snapshot),
        })),
      };
    },
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
