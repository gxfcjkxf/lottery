import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  createSettlementPreviewApi,
  settlementPreviewPermissions,
  type SettlementPreview,
} from "./settlement-preview-api";

const brand = "00000000-0000-4000-8000-000000000001";
const order = "00000000-0000-4000-8000-000000000002";
const game = "00000000-0000-4000-8000-000000000003";
const period = "00000000-0000-4000-8000-000000000004";
const drawId = "00000000-0000-4000-8000-000000000005";
const previewId = "00000000-0000-4000-8000-000000000006";
const admin = "00000000-0000-4000-8000-000000000007";
const audit = "00000000-0000-4000-8000-000000000008";
const hash = "a".repeat(64);
const draw = { regular: [1, 2], special: [3], digits: [] };

function envelope(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: status < 400, data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function preview(overrides: Partial<SettlementPreview> = {}): SettlementPreview {
  return {
    id: previewId,
    brand_id: brand,
    game_id: game,
    period_id: period,
    order_id: order,
    order_version: 4,
    order_status: "placed",
    period_version: 7,
    period_status: "drawn",
    draw_result_id: drawId,
    definition_hash: hash,
    draw_hash: hash,
    draw,
    outcome: "won",
    error_code: null,
    calculation: {
      won: true,
      combination_count: 2,
      multiplier: "9007199254740993",
      bet_points: "4",
      prize_points: "5",
      raw_prize_points: "5",
      capped_prize_points: "5",
    },
    created_by: admin,
    created_at: "2026-10-06T04:00:00Z",
    reason: "support review",
    audit_log_id: audit,
    current: true,
    applied: false,
    ...overrides,
  };
}

function account(overrides: Partial<AdminAccount> = {}): AdminAccount {
  return {
    id: admin,
    super_admin: false,
    brand_ids: [brand],
    permissions: [],
    ...overrides,
  };
}

describe("settlement preview permissions", () => {
  it("requires mapped staff grants for the selected brand", () => {
    expect(
      settlementPreviewPermissions(
        account({
          permissions_by_brand: {
            [brand]: ["settlement.view.brand", "settlement.preview.brand"],
          },
          platform_permissions: ["settlement.view.platform"],
        }),
        brand,
      ),
    ).toEqual({ view: true, preview: true });
    expect(
      settlementPreviewPermissions(
        account({
          super_admin: true,
          permissions: ["settlement.view.brand", "settlement.preview.brand"],
          platform_permissions: [],
        }),
        brand,
      ),
    ).toEqual({ view: false, preview: false });
    expect(
      settlementPreviewPermissions(
        account({ super_admin: true, platform_permissions: ["settlement.view.platform"] }),
        brand,
      ),
    ).toEqual({ view: false, preview: false });
    expect(
      settlementPreviewPermissions(
        account({ permissions: ["settlement.view.platform"] }),
        brand,
      ),
    ).toEqual({ view: false, preview: false });
    expect(
      settlementPreviewPermissions(
        account({ brand_ids: [], permissions: ["settlement.view.brand", "settlement.preview.brand"] }),
        brand,
      ),
    ).toEqual({ view: false, preview: false });
    expect(
      settlementPreviewPermissions(
        account({ id: "not-a-uuid", permissions: ["settlement.view.brand"] }),
        brand,
      ),
    ).toEqual({ view: false, preview: false });
  });
});

describe("settlement preview API", () => {
  it("fetches order context with brand and verifies its scope and preview eligibility", async () => {
    const context = {
      brand_id: brand,
      game_id: game,
      period_id: period,
      order_id: order,
      order_version: 4,
      order_status: "placed",
      definition_hash: hash,
      period_version: 7,
      period_status: "drawn",
      draw_result_id: drawId,
      draw_hash: hash,
      draw,
      can_preview: true,
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(context));
    await expect(createSettlementPreviewApi({ fetch: fetcher }).context(brand, order)).resolves.toEqual(context);
    expect(fetcher.mock.calls[0][0]).toBe(
      `/api/v1/admin/bet-orders/${order}/settlement-context`,
    );
    expect(fetcher.mock.calls[0][1]?.credentials).toBe("same-origin");
    expect((fetcher.mock.calls[0][1]?.headers as Headers).get("X-Brand-ID")).toBe(brand);
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope({ ...context, brand_id: "00000000-0000-4000-8000-000000000099" }),
        ),
      }).context(brand, order),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope({
            ...context,
            period_status: "open",
            draw_result_id: null,
            draw_hash: null,
            draw: null,
            can_preview: false,
          }),
        ),
      }).context(brand, order),
    ).resolves.toMatchObject({ can_preview: false, draw_result_id: null });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope({ ...context, period_status: "open", can_preview: true }),
        ),
      }).context(brand, order),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("sends the exact create body, cookie scope and caller-owned idempotency key", async () => {
    const body = {
      version: 4,
      period_version: 7,
      draw_result_id: drawId,
      reason: "support review",
    };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(envelope(preview(), 201));
    await createSettlementPreviewApi({ fetch: fetcher }).create(
      brand,
      order,
      body,
      "retry-key-1",
      admin,
    );
    const [url, init] = fetcher.mock.calls[0];
    const headers = init?.headers as Headers;
    expect(url).toBe(`/api/v1/admin/bet-orders/${order}/settlement-previews`);
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("same-origin");
    expect(headers.get("X-Brand-ID")).toBe(brand);
    expect(headers.get("Idempotency-Key")).toBe("retry-key-1");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(init?.body))).toEqual(body);
  });

  it("rejects receipts that disagree with requested versions, draw, order or creator", async () => {
    const body = {
      version: 4,
      period_version: 7,
      draw_result_id: drawId,
      reason: "support review",
    };
    const makeApi = (data: unknown) =>
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(data, 201)),
      });
    await expect(makeApi(preview({ order_version: 5 })).create(brand, order, body, "key"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(makeApi(preview({ draw_result_id: "00000000-0000-4000-8000-000000000099" })).create(brand, order, body, "key"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(makeApi(preview({ order_id: "00000000-0000-4000-8000-000000000099" })).create(brand, order, body, "key"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      makeApi(preview({ created_by: "00000000-0000-4000-8000-000000000099" })).create(
        brand,
        order,
        body,
        "key",
        admin,
      ),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(makeApi(preview({ reason: "changed" })).create(brand, order, body, "key"))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(preview(), 200)),
      }).create(brand, order, body, "key"),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("preserves large exact values and rejects noncanonical rationals and unsafe JSON integers", async () => {
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(preview())),
      }).preview(brand, previewId),
    ).resolves.toMatchObject({ calculation: { multiplier: "9007199254740993" } });
    for (const raw of ["01", "2/4", "1/1", "1.5", "-1"]) {
      await expect(
        createSettlementPreviewApi({
          fetch: vi.fn<typeof fetch>().mockResolvedValue(
            envelope(preview({
              calculation: { ...preview().calculation!, raw_prize_points: raw },
            })),
          ),
        }).preview(brand, previewId),
      ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope(preview({ order_version: Number.MAX_SAFE_INTEGER + 1 })),
        ),
      }).preview(brand, previewId),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope(preview({
            calculation: {
              ...preview().calculation!,
              raw_prize_points: "3/2",
              capped_prize_points: "3/2",
              prize_points: "2",
            },
          })),
        ),
      }).preview(brand, previewId),
    ).resolves.toMatchObject({ calculation: { prize_points: "2" } });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope(preview({
            calculation: {
              ...preview().calculation!,
              raw_prize_points: "3",
              capped_prize_points: "4",
              prize_points: "4",
            },
          })),
        ),
      }).preview(brand, previewId),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope(preview({
            outcome: "lost",
            calculation: {
              ...preview().calculation!,
              won: false,
              raw_prize_points: "0",
              capped_prize_points: "0",
              prize_points: "0",
            },
          })),
        ),
      }).preview(brand, previewId),
    ).resolves.toMatchObject({ outcome: "lost", calculation: { prize_points: "0" } });
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(
          envelope(preview({ created_at: "2026-02-30T12:00:00Z" })),
        ),
      }).preview(brand, previewId),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("treats malformed success responses as invalid and network failures as unknown", async () => {
    const malformed = createSettlementPreviewApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({})),
    });
    await expect(malformed.preview(brand, previewId)).rejects.toBeInstanceOf(AdminApiError);
    await expect(malformed.preview(brand, previewId)).rejects.toMatchObject({
      status: 502,
      code: "INVALID_RESPONSE",
    });
    const network = createSettlementPreviewApi({
      fetch: vi.fn<typeof fetch>().mockRejectedValue(new TypeError("offline")),
    });
    await expect(network.preview(brand, previewId)).rejects.toMatchObject({
      status: 0,
      code: "NETWORK_ERROR",
    });
  });

  it("enforces outcome status semantics, draw bounds, unique history IDs and hit trace invariants", async () => {
    const getPreview = (value: unknown) =>
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(value)),
      }).preview(brand, previewId);
    await expect(
      getPreview(preview({ outcome: "won", order_status: "cancelled" })),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    await expect(
      getPreview(preview({
        outcome: "excluded",
        order_status: "cancelled",
        calculation: null,
        error_code: "ORDER_NOT_PLACED",
      })),
    ).resolves.toMatchObject({ outcome: "excluded" });
    await expect(
      getPreview(preview({ draw: { ...draw, digits: [10] } })),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });

    const history = createSettlementPreviewApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope({
        brand_id: brand,
        order_id: order,
        items: [preview(), preview()],
        limit: 20,
        offset: 0,
        has_more: false,
      })),
    });
    await expect(history.history(brand, order)).rejects.toMatchObject({
      status: 502,
      code: "INVALID_RESPONSE",
    });

    const linePayload = (hit: unknown) => ({
      brand_id: brand,
      preview_id: previewId,
      order_id: order,
      items: [{
        selection: { regular: null, special: null, digits: null, exclude: null, attributes: null, features: null },
        hits: [hit],
        points: "1",
      }],
      limit: 20,
      offset: 0,
      has_more: false,
      total: 1,
    });
    const badHit = {
      code: "tier-1",
      exclusive: false,
      matched: false,
      selected: true,
      raw_points: "1",
      capped_points: "1",
      points: "1",
      trace: { op: "equals", actual: 1, matched: false },
    };
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(linePayload(badHit))),
      }).lines(brand, previewId),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    const deepTrace: Record<string, unknown> = { op: "leaf", actual: 0, matched: true };
    let cursor = deepTrace;
    for (let depth = 1; depth < 33; depth += 1) {
      const child: Record<string, unknown> = { op: "all", actual: 0, matched: true };
      cursor.children = [child];
      cursor = child;
    }
    const tooDeep = {
      ...badHit,
      matched: true,
      selected: true,
      trace: deepTrace,
    };
    await expect(
      createSettlementPreviewApi({
        fetch: vi.fn<typeof fetch>().mockResolvedValue(envelope(linePayload(tooDeep))),
      }).lines(brand, previewId),
    ).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("validates pagination bounds and line trace shape without numeric coercion", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      envelope({
        brand_id: brand,
        preview_id: previewId,
        order_id: order,
        items: [],
        limit: 20,
        offset: 0,
        has_more: false,
        total: 0,
      }),
    );
    const api = createSettlementPreviewApi({ fetch: fetcher });
    await expect(api.lines(brand, previewId)).resolves.toMatchObject({ total: 0 });
    await expect(api.lines(brand, previewId, 101)).rejects.toMatchObject({ code: "INVALID_INPUT" });
    const exactLarge = createSettlementPreviewApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(
        envelope({
          brand_id: brand,
          preview_id: previewId,
          order_id: order,
          items: [{
            selection: { regular: null, special: null, digits: null, exclude: null, attributes: null, features: null },
            hits: [],
            points: "900719925474099300000",
          }],
          limit: 20,
          offset: 0,
          has_more: false,
          total: 1,
        }),
      ),
    });
    await expect(exactLarge.lines(brand, previewId)).resolves.toMatchObject({
      items: [{ points: "900719925474099300000" }],
    });
  });
});
