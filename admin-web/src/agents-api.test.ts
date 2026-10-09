import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import {
  agentsPermissions,
  createAgentsApi,
  standardAdminApiError,
  type AgentNode,
  type AgentPolicy,
  type NodeConfig,
  type PolicyConfig,
  type Revision,
} from "./agents-api";

const brand = "00000000-0000-4000-8000-000000000001";
const accountId = "00000000-0000-4000-8000-000000000002";
const member = "00000000-0000-4000-8000-000000000003";
const root = "00000000-0000-4000-8000-000000000004";
const child = "00000000-0000-4000-8000-000000000005";
const actor = "00000000-0000-4000-8000-000000000006";
const at = "2026-10-06T04:00:00.123456Z";

const policyConfig: PolicyConfig = {
  enabled: true,
  max_depth: 5,
  ratio_cap: "0.25",
  mode: "loss",
  cycle: "monthly",
};
const nodeConfig: NodeConfig = {
  ratio: "0.1",
  mode: null,
  status: "active",
  can_create_children: true,
};
const policy: AgentPolicy = {
  brand_id: brand,
  version: 8,
  config: policyConfig,
  updated_at: at,
};
function agentNode(overrides: Partial<AgentNode> = {}): AgentNode {
  return {
    id: root,
    brand_id: brand,
    member_id: member,
    parent_id: null,
    depth: 1,
    path: [root],
    version: 3,
    config: nodeConfig,
    effective_mode: "loss",
    mode_source_agent_id: null,
    policy_version: 8,
    parent_version: null,
    created_by: actor,
    created_at: at,
    updated_at: at,
    ...overrides,
  };
}
function ok(data: unknown, status: number = 200): Response {
  return new Response(JSON.stringify({ success: true, data }), { status });
}
function fail(status: number, code = "AGENT_VERSION_CONFLICT") {
  return new Response(JSON.stringify({ success: false, error: { code, message: "request rejected" } }), { status });
}

describe("agent permissions", () => {
  const account = (overrides: Partial<AdminAccount> = {}): AdminAccount => ({
    id: accountId,
    super_admin: false,
    brand_ids: [brand],
    permissions: [],
    ...overrides,
  });

  it("requires mapped grants for a member and denies flat or platform grants", () => {
    const grants = ["agent_policy.view.brand", "agent_policy.write.brand", "agent.view.brand", "agent.write.brand"];
    expect(agentsPermissions(account({ permissions_by_brand: { [brand]: grants } }), brand)).toEqual({
      policyView: true, policyWrite: true, view: true, write: true,
    });
    expect(agentsPermissions(account({ permissions_by_brand: { [brand]: grants }, brand_ids: [] }), brand)).toEqual({
      policyView: false, policyWrite: false, view: false, write: false,
    });
    expect(agentsPermissions(account({ permissions: grants, permissions_by_brand: {} }), brand).view).toBe(false);
    expect(agentsPermissions(account({ super_admin: true, platform_permissions: ["agent.view.platform", "agent_policy.view.platform"], permissions_by_brand: { [brand]: grants } }), brand)).toEqual({
      policyView: false, policyWrite: false, view: false, write: false,
    });
    expect(agentsPermissions(account({ super_admin: true }), brand).view).toBe(false);
    expect(agentsPermissions(account({ platform_permissions: ["agent.view.platform"] }), brand).view).toBe(false);
    expect(agentsPermissions(account({ permissions: grants }), brand).view).toBe(false);
  });

  it("maps unknown errors while preserving known admin errors", () => {
    const known = new AdminApiError("conflict", 409, "AGENT_VERSION_CONFLICT");
    expect(standardAdminApiError(known)).toBe(known);
    expect(standardAdminApiError(new Error("offline"))).toMatchObject({ status: 0, code: "NETWORK_ERROR", message: "offline" });
  });
});

describe("agents admin API", () => {
  it("gets scoped policy and history without rewriting GETs or adding internal context", async () => {
    const revision: Revision = {
      brand_id: brand, agent_id: null, version: 8, config: policyConfig,
      actor_type: "admin", actor_id: actor, reason: "reviewed", created_at: at, audit_log_id: null,
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(policy))
      .mockResolvedValueOnce(ok({ brand_id: brand, agent_id: null, items: [revision], limit: 10, offset: 2, total_count: "12" }));
    const api = createAgentsApi(fetcher);
    await expect(api.policy(brand)).resolves.toEqual(policy);
    await expect(api.policyHistory(brand, 10, 2)).resolves.toMatchObject({ items: [revision], total_count: "12" });
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/agent-policy");
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/agent-policy");
    expect(init?.method).toBe("GET");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);
    expect(new Headers(init?.headers).get("Authorization")).toBeNull();
    expect(init?.body).toBeUndefined();
    expect(fetcher.mock.calls[1][0]).toBe("/api/v1/admin/agent-policy/history?limit=10&offset=2");
  });

  it("lists roots or an exact parent page and validates response identity and structure", async () => {
    const childNode = agentNode({ id: child, member_id: actor, parent_id: root, depth: 2,
      path: [root, child], parent_version: 3, mode_source_agent_id: child, effective_mode: "turnover",
      config: { ...nodeConfig, mode: "turnover" } });
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok({ brand_id: brand, parent_id: null, items: [agentNode()], limit: 20, offset: 0, total_count: "1" }))
      .mockResolvedValueOnce(ok({ brand_id: brand, parent_id: root, items: [childNode], limit: 5, offset: 5, total_count: "7" }));
    const api = createAgentsApi(fetcher);
    await expect(api.tree(brand)).resolves.toMatchObject({ parent_id: null, items: [agentNode()] });
    await expect(api.tree(brand, root, 5, 5)).resolves.toMatchObject({ parent_id: root, items: [childNode] });
    expect(fetcher.mock.calls[0][0]).toBe("/api/v1/admin/agents/tree?limit=20&offset=0");
    expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/admin/agents/tree?limit=5&offset=5&parent_id=${root}`);

    for (const invalid of [
      { ...agentNode(), brand_id: child },
      { ...agentNode(), path: [child] },
      { ...agentNode(), depth: 2 },
      { ...agentNode(), parent_id: child },
      { ...agentNode(), updated_at: "not-a-date" },
    ]) {
      await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(ok({
        brand_id: brand, parent_id: null, items: [invalid], limit: 20, offset: 0, total_count: "1",
      }))).tree(brand)).rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
    }
  });

  it("reads a node/history only when backend identity and page envelope match", async () => {
    const revision: Revision = {
      brand_id: brand, agent_id: root, version: 3, config: nodeConfig,
      actor_type: "user", actor_id: actor, reason: "changed ratio", created_at: at, audit_log_id: null,
    };
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(ok(agentNode()))
      .mockResolvedValueOnce(ok({ brand_id: brand, agent_id: root, items: [revision], limit: 20, offset: 0, total_count: "1" }));
    const api = createAgentsApi(fetcher);
    await expect(api.node(brand, root)).resolves.toEqual(agentNode());
    await expect(api.nodeHistory(brand, root)).resolves.toMatchObject({ agent_id: root, items: [revision] });
    expect(fetcher.mock.calls[0][0]).toBe(`/api/v1/admin/agents/${root}`);
    expect(fetcher.mock.calls[1][0]).toBe(`/api/v1/admin/agents/${root}/history?limit=20&offset=0`);
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(ok(agentNode({ id: child })))).node(brand, root))
      .rejects.toMatchObject({ status: 502, code: "INVALID_RESPONSE" });
  });

  it("saves policy with an unchanged request body, required key and exact next-version receipt", async () => {
    const body = { version: 8, config: { ...policyConfig, ratio_cap: "0.25" }, reason: "quarterly review" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok({ ...policy, version: 9, config: body.config }));
    await expect(createAgentsApi(fetcher).savePolicy(brand, body, "policy-key"))
      .resolves.toMatchObject({ version: 9, config: body.config });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/agent-policy");
    expect(init?.method).toBe("PUT");
    expect(init?.credentials).toBe("same-origin");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("policy-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    for (const badRatio of ["0.10", "0.1234567", "1.0", "1.1", "-0.1", "0.000000"]) {
      await expect(createAgentsApi(vi.fn<typeof fetch>()).savePolicy(brand,
        { ...body, config: { ...body.config, ratio_cap: badRatio } }, "key"))
        .rejects.toMatchObject({ status: 0, code: "AGENT_INPUT_INVALID" });
    }
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...policy, version: 10, config: body.config })))
      .savePolicy(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(ok({ ...policy, version: 9,
      config: { ...body.config, ratio_cap: "0.3" } })))
      .savePolicy(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("creates with CAS fields and accepts only the exact first receipt", async () => {
    const body = { policy_version: 8, member_id: member, parent_id: null, parent_version: null,
      config: nodeConfig, reason: "initial agent" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(agentNode({ version: 1 }), 201));
    await expect(createAgentsApi(fetcher).createNode(brand, body, "create-key"))
      .resolves.toMatchObject({ id: root, version: 1 });
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("/api/v1/admin/agents");
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("create-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(ok(agentNode({ version: 2 }))))
      .createNode(brand, body, "key")).rejects.toMatchObject({ status: 0, code: "INVALID_RESPONSE" });
  });

  it("updates with exactly one PUT and never performs an implicit GET", async () => {
    const body = { version: 3, policy_version: 8, parent_version: null,
      config: { ...nodeConfig, ratio: "0.2" }, reason: "adjusted root rate" };
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(ok(agentNode({ version: 4, config: body.config })));
    await expect(createAgentsApi(fetcher).updateNode(brand, root, body, "update-key"))
      .resolves.toMatchObject({ version: 4, config: body.config });
    expect(fetcher).toHaveBeenCalledTimes(1);
    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe(`/api/v1/admin/agents/${root}`);
    expect(init?.method).toBe("PUT");
    expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("update-key");
    expect(JSON.parse(String(init?.body))).toEqual(body);
  });

  it("rejects invalid inputs before transport and preserves backend conflict codes", async () => {
    const fetcher = vi.fn<typeof fetch>();
    const api = createAgentsApi(fetcher);
    await expect(api.tree(brand, null, 101)).rejects.toMatchObject({ code: "AGENT_INPUT_INVALID" });
    await expect(api.tree(brand, "bad-id")).rejects.toMatchObject({ code: "AGENT_INPUT_INVALID" });
    await expect(api.node(brand, "bad-id")).rejects.toMatchObject({ code: "AGENT_INPUT_INVALID" });
    await expect(api.nodeHistory(brand, root, 20, -1)).rejects.toMatchObject({ code: "AGENT_INPUT_INVALID" });
    expect(fetcher).not.toHaveBeenCalled();
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockResolvedValue(fail(409)))
      .savePolicy(brand, { version: 8, config: policyConfig, reason: "review" }, "key"))
      .rejects.toMatchObject({ status: 409, code: "AGENT_VERSION_CONFLICT" });
    await expect(createAgentsApi(vi.fn<typeof fetch>().mockRejectedValue(new Error("offline")))
      .policy(brand)).rejects.toMatchObject({ status: 0, code: "NETWORK_ERROR" });
  });
});
