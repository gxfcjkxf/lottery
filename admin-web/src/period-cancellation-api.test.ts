import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createPeriodCancellationApi,
  periodCancellationPermissions,
} from "./period-cancellation-api";

const brand = "brand-a";
const cancellation = {
  id: "cancel-1",
  brand_id: brand,
  game_id: "game-1",
  period_id: "period/1",
  period_version: 4,
  mode: "bet_cancelled",
  cause: "operator_cancel",
  state: "processing",
  version: 1,
  reason: "operator decision",
  created_by: "admin-1",
  created_at: "2026-10-06T00:00:00Z",
  completed_at: null,
  last_error_code: "",
  total_count: 3,
  pending_count: 2,
  refunded_count: 1,
  already_refunded_count: 0,
  failed_count: 0,
};
const ok = (data: unknown, status = 200) =>
  new Response(JSON.stringify({ success: true, data }), { status });

describe("period cancellation API", () => {
  it("reads exact primary period identity and version separately from cancellation task version", async () => {
    const period = {
      id: "period-1",
      brand_id: brand,
      game_id: "game-1",
      period_no: "P001",
      status: "betting",
      sequence: 1,
      version: 2,
      bet_start_at: "2026-10-06T00:00:00Z",
      bet_end_at: "2026-10-06T01:00:00Z",
      draw_at: "2026-10-06T02:00:00Z",
    };
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok(period))
      .mockResolvedValueOnce(ok({ ...period, brand_id: "other" }));
    const api = createPeriodCancellationApi(fetcher);
    expect(await api.getPeriod(brand, "period-1")).toEqual(period);
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/periods/period-1");
    await expect(api.getPeriod(brand, "period-1")).rejects.toMatchObject({
      status: 502,
    });
  });
  it("rejects impossible progress counts and completion claims", async () => {
    for (const invalid of [
      { ...cancellation, total_count: 4 },
      {
        ...cancellation,
        state: "completed",
        completed_at: "2026-10-06T01:00:00Z",
      },
      { ...cancellation, state: "processing", last_error_code: "FAILURE" },
    ]) {
      await expect(
        createPeriodCancellationApi(
          vi
            .fn<typeof fetch>()
            .mockResolvedValue(ok({ cancellation: invalid })),
        ).getCancellation(brand, "p"),
      ).rejects.toMatchObject({ status: 502 });
    }
  });
  it("uses cookie credentials, explicit brand, encoded UUID paths, exact body and supplied keys", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ cancellation }))
      .mockResolvedValueOnce(ok(cancellation, 202))
      .mockResolvedValueOnce(ok(cancellation, 202));
    const api = createPeriodCancellationApi(fetcher);
    expect(await api.getCancellation(brand, "period/1")).toEqual({
      cancellation,
    });
    expect(
      await api.cancelPeriod(
        brand,
        "period/1",
        {
          version: 4,
          mode: "bet_cancelled",
          cause: "operator_cancel",
          reason: "operator decision",
        },
        "cancel-key",
      ),
    ).toEqual(cancellation);
    expect(
      await api.retryCancellation(
        brand,
        "period/1",
        { version: 1, reason: "retry" },
        "retry-key",
      ),
    ).toEqual(cancellation);
    expect(
      fetcher.mock.calls.map(([url, init]) => [
        url,
        init?.method,
        init?.body,
        new Headers(init?.headers).get("Idempotency-Key"),
      ]),
    ).toEqual([
      ["/api/v1/admin/periods/period%2F1/cancellation", "GET", undefined, null],
      [
        "/api/v1/admin/periods/period%2F1/cancel",
        "POST",
        JSON.stringify({
          version: 4,
          mode: "bet_cancelled",
          cause: "operator_cancel",
          reason: "operator decision",
        }),
        "cancel-key",
      ],
      [
        "/api/v1/admin/periods/period%2F1/cancellation/retry",
        "POST",
        JSON.stringify({ version: 1, reason: "retry" }),
        "retry-key",
      ],
    ]);
    for (const [, init] of fetcher.mock.calls) {
      expect(init?.credentials).toBe("same-origin");
      expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    }
  });

  it("accepts null reads and completed 200 writes; rejects malformed DTOs and mismatched brands as 502", async () => {
    expect(
      await createPeriodCancellationApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok({ cancellation: null })),
      ).getCancellation(brand, "p"),
    ).toEqual({ cancellation: null });
    const completed = {
      ...cancellation,
      state: "completed",
      completed_at: "2026-10-06T01:00:00Z",
      pending_count: 0,
      refunded_count: 3,
    };
    expect(
      await createPeriodCancellationApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok(completed)),
      ).cancelPeriod(
        brand,
        "p",
        {
          version: 4,
          mode: "bet_cancelled",
          cause: "no_result",
          reason: "no result",
        },
        "key",
      ),
    ).toEqual(completed);
    for (const invalid of [
      { ...cancellation, total_count: 1.5 },
      { ...cancellation, refunded_count: -1 },
      { ...cancellation, brand_id: "other" },
    ]) {
      await expect(
        createPeriodCancellationApi(
          vi.fn<typeof fetch>().mockResolvedValue(ok(invalid)),
        ).cancelPeriod(
          brand,
          "p",
          {
            version: 4,
            mode: "bet_cancelled",
            cause: "operator_cancel",
            reason: "x",
          },
          "key",
        ),
      ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("preserves structured errors and maps malformed successful responses to 502", async () => {
    const failure = new Response(
      JSON.stringify({
        success: false,
        error: { code: "CONFLICT", message: "stale" },
      }),
      { status: 409 },
    );
    await expect(
      createPeriodCancellationApi(
        vi.fn<typeof fetch>().mockResolvedValue(failure),
      ).getCancellation(brand, "p"),
    ).rejects.toMatchObject({
      status: 409,
      code: "CONFLICT",
      message: "stale",
    });
    await expect(
      createPeriodCancellationApi(
        vi
          .fn<typeof fetch>()
          .mockResolvedValue(new Response("bad", { status: 200 })),
      ).getCancellation(brand, "p"),
    ).rejects.toBeInstanceOf(AdminApiError);
    await expect(
      createPeriodCancellationApi(
        vi.fn<typeof fetch>().mockResolvedValue(ok({})),
      ).getCancellation(brand, "p"),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("calculates explicit view and write permissions, including super-admin denial", () => {
    const account: AdminAccount = {
      id: "a",
      super_admin: false,
      brand_ids: [brand],
      permissions: [],
      platform_permissions: ["period.view.platform"],
      permissions_by_brand: {
        [brand]: ["period.view.brand", "period.cancel.brand", "period.cancel_retry.brand"],
      },
    };
    expect(periodCancellationPermissions(account, brand)).toEqual({
      view: true,
      cancel: true,
      retry: true,
    });
    expect(
      periodCancellationPermissions({ ...account, super_admin: true }, brand),
    ).toEqual({ view: false, cancel: false, retry: false });
    expect(
      periodCancellationPermissions(
        {
          ...account,
          platform_permissions: [],
          permissions_by_brand: { [brand]: ["period.cancel.brand"] },
        },
        brand,
      ),
    ).toEqual({ view: false, cancel: true, retry: false });
  });
});
