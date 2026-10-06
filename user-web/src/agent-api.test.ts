import { describe, expect, it, vi } from "vitest";
import { createAgentApi, type AgentNode } from "./agent-api";

const brand = "11111111-1111-4111-8111-111111111111";
const member = "22222222-2222-4222-8222-222222222222";
const root = "33333333-3333-4333-8333-333333333333";
const selfId = "44444444-4444-4444-8444-444444444444";
const childId = "55555555-5555-4555-8555-555555555555";
const creator = "66666666-6666-4666-8666-666666666666";

function node(overrides: Partial<AgentNode> = {}): AgentNode {
  return {
    id: selfId, brand_id: brand, member_id: member, parent_id: root,
    depth: 2, path: [root, selfId], version: 3,
    config: { ratio: "0.1", mode: null, status: "active", can_create_children: true },
    effective_mode: "loss", mode_source_agent_id: root, policy_version: 4,
    parent_version: 2, created_by: creator,
    created_at: "2026-10-06T00:00:00Z", updated_at: "2026-10-06T00:00:00Z",
    ...overrides,
  };
}
const envelope = (data: unknown, status = 200) => new Response(JSON.stringify({ success: true, data }), { status });
const child = (overrides: Partial<AgentNode> = {}) => node({
  id: childId, member_id: "77777777-7777-4777-8777-777777777777",
  parent_id: selfId, depth: 3, path: [root, selfId, childId], parent_version: 3,
  ...overrides,
});

describe("public agent API", () => {
  it("uses public cookie-authenticated endpoints, pagination and validates actor scope", async () => {
    const fetchImpl = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(envelope(node()))
      .mockResolvedValueOnce(envelope({ brand_id: brand, parent_id: selfId, items: [child()], limit: 20, offset: 0, total_count: "1" }));
    const api = createAgentApi({ brandCode: "north star", fetchImpl });
    await expect(api.me()).resolves.toMatchObject({ id: selfId, brand_id: brand, member_id: member });
    const page = await api.children();
    expect(page.items[0].parent_id).toBe(selfId);
    expect(fetchImpl.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/b/north%20star/agent/me",
      "/api/v1/b/north%20star/agent/children?limit=20&offset=0",
    ]);
    for (const [, init] of fetchImpl.mock.calls) {
      expect(init?.credentials).toBe("include");
      const headers = new Headers(init?.headers);
      expect(headers.get("X-Brand-ID")).toBeNull();
      expect(headers.get("Authorization")).toBeNull();
      expect(headers.get("Cookie")).toBeNull();
    }
    expect(fetchImpl.mock.calls[1][1]?.method).toBe("GET");
  });

  it("rejects malformed paths, cross-brand children and invalid pagination", async () => {
    const fetchImpl = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(envelope(node({ path: [selfId] })))
      .mockResolvedValueOnce(envelope({ brand_id: brand, parent_id: selfId, items: [child({ brand_id: root })], limit: 20, offset: 0, total_count: "1" }));
    const api = createAgentApi({ fetchImpl });
    await expect(api.me()).rejects.toMatchObject({ code: "invalid_response", status: 502 });
    await expect(api.children()).rejects.toMatchObject({ code: "invalid_response", status: 502 });
    await expect(api.children(101)).rejects.toMatchObject({ code: "invalid_parameter", status: 0 });
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it("sends a frozen-shape update body and validates its receipt and idempotency key", async () => {
    const updated = child({ version: 4, config: { ...child().config, ratio: "0.25", mode: "turnover" } });
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(envelope(updated));
    const api = createAgentApi({ fetchImpl });
    const body = { version: 3, policy_version: 4, parent_version: 3, ratio: "0.25", mode: "turnover" as const, reason: "Reviewed with child" };
    await expect(api.updateChild(childId, body, "update-child-0001", { brandId: brand, memberId: member, parentId: selfId, childMemberId: "77777777-7777-4777-8777-777777777777" })).resolves.toMatchObject({ id: childId, member_id: "77777777-7777-4777-8777-777777777777", version: 4 });
    expect(fetchImpl.mock.calls[0][0]).toBe(`/api/v1/agent/children/${childId}/config`);
    const [, init] = fetchImpl.mock.calls[0];
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("include");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    const headers = new Headers(init?.headers);
    expect(headers.get("Idempotency-Key")).toBe("update-child-0001");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Brand-ID")).toBeNull();
  });

  it.each([
    ["child id", () => child({ id: "88888888-8888-4888-8888-888888888888", path: [root, selfId, "88888888-8888-4888-8888-888888888888"] })],
    ["ratio", () => child({ config: { ...child().config, ratio: "0.3" } })],
    ["mode", () => child({ config: { ...child().config, mode: "loss" } })],
    ["child version", () => child({ version: 9 })],
    ["policy version", () => child({ policy_version: 5 })],
    ["parent version", () => child({ parent_version: 4 })],
    ["brand", () => child({ brand_id: "99999999-9999-4999-8999-999999999999" })],
    ["parent", () => child({ parent_id: "88888888-8888-4888-8888-888888888888", path: [root, "88888888-8888-4888-8888-888888888888", childId] })],
    ["child member id", () => child({ member_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" })],
  ])("keeps a mismatched %s response unknown and never exposes it as a receipt", async (_label, makeNode) => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(envelope((makeNode as () => AgentNode)()));
    const api = createAgentApi({ fetchImpl });
    const error = await api.updateChild(childId, { version: 3, policy_version: 4, parent_version: 3, ratio: "0.2", mode: null, reason: "Review" }, "update-child-0002", { brandId: brand, memberId: member, parentId: selfId, childMemberId: "77777777-7777-4777-8777-777777777777" }).catch((cause: unknown) => cause);
    expect(error).toMatchObject({ status: 502 });
    expect(error).not.toHaveProperty("receipt");
  });

  it("matches the child by its own member ID, not the caller's member ID", async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(envelope(child({ version: 4, config: { ...child().config, ratio: "0.2" } })));
    const api = createAgentApi({ fetchImpl });
    await expect(api.updateChild(childId, { version: 3, policy_version: 4, parent_version: 3, ratio: "0.2", mode: null, reason: "Review" }, "update-child-0003", { brandId: brand, memberId: member, parentId: selfId, childMemberId: "77777777-7777-4777-8777-777777777777" })).resolves.toMatchObject({ member_id: "77777777-7777-4777-8777-777777777777" });
  });

  it("rejects noncanonical fractions and malformed idempotency keys before sending", async () => {
    const fetchImpl = vi.fn<typeof fetch>();
    const api = createAgentApi({ fetchImpl });
    const body = { version: 1, policy_version: 1, parent_version: 1, ratio: "0.10", mode: null, reason: "Review" };
    await expect(api.updateChild(childId, body, "valid-key-0001")).rejects.toMatchObject({ code: "invalid_parameter" });
    await expect(api.updateChild(childId, { ...body, ratio: "0.1" }, "bad/key")).rejects.toMatchObject({ code: "invalid_parameter" });
    expect(fetchImpl).not.toHaveBeenCalled();
  });
});
