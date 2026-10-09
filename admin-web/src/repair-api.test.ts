import { describe, it, expect, vi } from "vitest";
import { createRepairApi } from "./repair-api";
describe("balance repair API", () => {
  it("encodes member, scopes brand, sends preview token rather than arbitrary points", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(
          JSON.stringify({ success: true, data: { id: "repair" } }),
          { status: 200 },
        ),
    );
    const api = createRepairApi(fetcher as typeof fetch);
    await api.repair(
      "brand-A",
      "member/id",
      { version: 4, token: "observation", reason: "rebuild" },
      "operation-key",
    );
    const [url, init] = fetcher.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(url).toContain("member%2Fid/repair");
    expect(init.credentials).toBe("same-origin");
    const headers = new Headers(init.headers);
    expect(headers.get("X-Brand-ID")).toBe("brand-A");
    expect(headers.get("Idempotency-Key")).toBe("operation-key");
    expect(JSON.parse(init.body as string)).toEqual({
      version: 4,
      token: "observation",
      reason: "rebuild",
    });
  });
  it("keeps preview and immutable history read-only", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify({ success: true, data: { items: [] } }), {
          status: 200,
        }),
    );
    const api = createRepairApi(fetcher as typeof fetch);
    await api.preview("a", "m");
    await api.history("a", "m");
    expect(
      fetcher.mock.calls.map(
        (c) => (c as unknown as [string, RequestInit])[1].method,
      ),
    ).toEqual(["GET", "GET"]);
  });
  it("preserves historical repair audit snapshots without bucket normalization", async () => {
    const buckets = Object.fromEntries(["recharge", "winning", "gift"].map((source) => [source, { available: "1", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }]));
    const snapshot = { version: 3, buckets };
    const row = { id: "repair", version: 3, before_snapshot: snapshot, after_snapshot: snapshot, reason: "legacy", actor_id: "actor", request_id: "request", created_at: "2026-01-01T00:00:00Z" };
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ success: true, data: { items: [row] } }), { status: 200 }));
    const result = await createRepairApi(fetcher as typeof fetch).history("brand", "member");
    expect(result.items[0]?.before_snapshot).toEqual(snapshot);
    expect(Object.keys(buckets)).toEqual(["recharge", "winning", "gift"]);
  });
  it("does not convert denied repair into success", async () => {
    const api = createRepairApi(
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              success: false,
              error: {
                code: "POINTS_OPERATION_CONFLICT",
                message: "预览已变化",
              },
            }),
            { status: 409 },
          ),
      ) as typeof fetch,
    );
    await expect(
      api.repair("a", "m", { version: 1, token: "stale", reason: "r" }, "key"),
    ).rejects.toMatchObject({ status: 409, code: "POINTS_OPERATION_CONFLICT" });
  });
});
