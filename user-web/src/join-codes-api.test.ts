import { describe, expect, it, vi } from "vitest";
import {
  createJoinCodesApi,
  JoinCodesApiError,
  normalizeJoinCode,
  type JoinCode,
} from "./join-codes-api";

const brandId = "11111111-1111-4111-8111-111111111111";
const memberId = "22222222-2222-4222-8222-222222222222";
const codeId = "33333333-3333-4333-8333-333333333333";
const agentId = "44444444-4444-4444-8444-444444444444";
const context = { brand_id: brandId, member_id: memberId };
const code: JoinCode = {
  id: codeId,
  brand_id: brandId,
  kind: "agent",
  code: "A1B2C3D4E5F607182930ABCD",
  owner_member_id: memberId,
  agent_id: agentId,
  status: "active",
  starts_at: null,
  expires_at: "2026-10-08T12:00:00.123Z",
  version: 2,
  usable: true,
  created_at: "2026-10-06T04:00:00Z",
  updated_at: "2026-10-06T04:00:00Z",
};
const page = (overrides: Record<string, unknown> = {}) => ({
  brand_id: brandId,
  member_id: memberId,
  items: [code],
  limit: 20,
  offset: 0,
  total_count: "1",
  ...overrides,
});
const attribution = (overrides: Record<string, unknown> = {}) => ({
  brand_id: brandId,
  member_id: memberId,
  join_method: "agent_code",
  joined_at: "2026-10-06T04:00:00Z",
  code_id: codeId,
  source_code: code.code,
  legacy: false,
  ...overrides,
});
const ok = (data: unknown) =>
  new Response(JSON.stringify({ success: true, data }), { status: 200 });

describe("user join codes API", () => {
  it("lists only the authenticated member's codes with strict scope and paging", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(page()));
    const api = createJoinCodesApi({ brandCode: "north star", fetch: fetcher });
    await expect(api.list(context)).resolves.toMatchObject({
      brand_id: brandId,
      member_id: memberId,
      items: [code],
      total_count: "1",
    });
    expect(fetcher.mock.calls[0]?.[0]).toBe(
      "/api/v1/b/north%20star/me/join-codes?limit=20&offset=0",
    );
    expect(fetcher.mock.calls[0]?.[1]).toMatchObject({
      method: "GET",
      credentials: "include",
    });
  });

  it("loads own attribution and keeps legacy membership clearly marked", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      ok(attribution({ join_method: "domain", code_id: null, source_code: null, legacy: true })),
    );
    const api = createJoinCodesApi({ fetch: fetcher });
    await expect(api.attribution(context)).resolves.toMatchObject({
      join_method: "domain",
      code_id: null,
      source_code: null,
      legacy: true,
    });
    expect(fetcher.mock.calls[0]?.[0]).toBe("/api/v1/me/attribution");
  });

  it("validates code, date, pagination and own-scope DTO fields", async () => {
    const invalidPages = [
      page({ items: [{ ...code, code: "abc123" }] }),
      page({ items: [{ ...code, starts_at: "2026-02-30T12:00:00Z" }] }),
      page({ items: [{ ...code, kind: "operator" }] }),
      page({ items: [{ ...code, agent_id: null }] }),
      page({ items: [{ ...code, admin_reason: "private" }] }),
      page({ brand_id: "55555555-5555-4555-8555-555555555555" }),
      page({ member_id: "55555555-5555-4555-8555-555555555555" }),
      page({ total_count: "01" }),
      page({ items: [{ ...code, starts_at: "2026-10-09T00:00:00Z", expires_at: "2026-10-05T00:00:00Z" }] }),
    ];
    const fetcher = vi.fn<typeof fetch>();
    for (const invalid of invalidPages) fetcher.mockResolvedValueOnce(ok(invalid));
    const api = createJoinCodesApi({ fetch: fetcher });
    for (let index = 0; index < invalidPages.length; index++) {
      await expect(api.list(context)).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });
    }

    const foreignAttributions = [
      attribution({ member_id: "55555555-5555-4555-8555-555555555555" }),
      attribution({ parent_id: "55555555-5555-4555-8555-555555555555" }),
    ];
    const foreignAttributionApi = createJoinCodesApi({
      fetch: vi.fn<typeof fetch>()
        .mockResolvedValueOnce(ok(foreignAttributions[0]))
        .mockResolvedValueOnce(ok(foreignAttributions[1])),
    });
    for (let index = 0; index < foreignAttributions.length; index++) {
      await expect(foreignAttributionApi.attribution(context)).rejects.toMatchObject({
        status: 502,
        code: "invalid_response",
      });
    }
  });

  it("rejects a context change before requesting and rejects malformed successful envelopes", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createJoinCodesApi({ fetch: fetcher });
    await expect(
      api.list({ brand_id: "bad", member_id: memberId }),
    ).rejects.toBeInstanceOf(JoinCodesApiError);
    expect(fetcher).not.toHaveBeenCalled();

    const malformed = createJoinCodesApi({
      fetch: vi.fn<typeof fetch>().mockResolvedValue(
        new Response(JSON.stringify({ success: true }), { status: 200 }),
      ),
    });
    await expect(malformed.list(context)).rejects.toMatchObject({
      status: 502,
      code: "invalid_response",
    });
  });

  it("normalizes explicit join codes and rejects anything outside 24 hex characters", () => {
    expect(normalizeJoinCode("  a1b2c3d4e5f607182930abcd\n")).toBe(
      "A1B2C3D4E5F607182930ABCD",
    );
    expect(() => normalizeJoinCode("short")).toThrow(/24 hexadecimal/i);
    expect(() => normalizeJoinCode("G1B2C3D4E5F607182930ABCD")).toThrow(
      /24 hexadecimal/i,
    );
  });
});
