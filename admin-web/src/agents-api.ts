import { AdminApiError, type AdminAccount } from "./admin-api";
import { brandPermissionSet } from "./brand-permissions";

const BASE = "/api/v1/admin";
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const MAX_PAGE_SIZE = 100;
const MAX_OFFSET = 1_000_000;

export type AgentMode = "loss" | "turnover";
export type AgentStatus = "active" | "disabled";

export interface PolicyConfig {
  enabled: boolean;
  max_depth: number;
  ratio_cap: string;
  mode: AgentMode;
  cycle: "weekly" | "monthly";
}

export interface NodeConfig {
  ratio: string;
  mode: AgentMode | null;
  status: AgentStatus;
  can_create_children: boolean;
}

export interface AgentPolicy {
  brand_id: string;
  version: number;
  config: PolicyConfig;
  updated_at: string;
  audit_log_id?: string | null;
}

export interface AgentNode {
  id: string;
  brand_id: string;
  member_id: string;
  parent_id: string | null;
  depth: number;
  path: string[];
  version: number;
  config: NodeConfig;
  effective_mode: AgentMode;
  mode_source_agent_id: string | null;
  policy_version: number;
  parent_version: number | null;
  created_by: string;
  created_at: string;
  updated_at: string;
  audit_log_id?: string | null;
}

export interface CreateAgentBody {
  policy_version: number;
  member_id: string;
  parent_id: string | null;
  parent_version: number | null;
  config: NodeConfig;
  reason: string;
}

export interface UpdateAgentBody {
  version: number;
  policy_version: number;
  parent_version: number | null;
  config: NodeConfig;
  reason: string;
}

export interface SaveAgentPolicyBody {
  version: number;
  config: PolicyConfig;
  reason: string;
}

export interface Revision {
  brand_id: string;
  agent_id: string | null;
  version: number;
  config: PolicyConfig | NodeConfig;
  actor_type: "system" | "admin" | "user";
  actor_id: string | null;
  reason: string;
  created_at: string;
  audit_log_id: string | null;
}

export interface AgentTree {
  brand_id: string;
  parent_id: string | null;
  items: AgentNode[];
  limit: number;
  offset: number;
  total_count: string;
}

export interface AgentRevisionPage {
  brand_id: string;
  agent_id: string | null;
  items: Revision[];
  limit: number;
  offset: number;
  total_count: string;
}

type Envelope = {
  success?: unknown;
  data?: unknown;
  error?: string | { code?: string; message?: string } | null;
};

export function standardAdminApiError(
  error: unknown,
  fallback = "Admin request failed",
): AdminApiError {
  if (error instanceof AdminApiError) return error;
  if (error instanceof Error)
    return new AdminApiError(error.message || fallback, 0, "NETWORK_ERROR");
  return new AdminApiError(fallback, 0, "NETWORK_ERROR");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
function validUuid(value: unknown): value is string {
  return typeof value === "string" && UUID_RE.test(value);
}
function positiveInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 1;
}
function nonnegativeInt(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) >= 0;
}
function canonicalRatio(value: unknown): value is string {
  return typeof value === "string" &&
    /^(?:0|1|0\.(?:[0-9]{0,5}[1-9]))$/.test(value) &&
    (value === "0" || value === "1" || value.split(".")[1].length <= 6);
}
function validMode(value: unknown): value is AgentMode {
  return value === "loss" || value === "turnover";
}
function validDate(value: unknown): value is string {
  if (typeof value !== "string") return false;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/.exec(value);
  if (!match) return false;
  const [, y, mo, d, h, mi, s, , , , oh, om] = match;
  const year = Number(y), month = Number(mo), day = Number(d);
  const days = month === 2
    ? (year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28)
    : [4, 6, 9, 11].includes(month) ? 30 : 31;
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > days ||
      Number(h) > 23 || Number(mi) > 59 || Number(s) > 59) return false;
  if (oh !== undefined && (Number(oh) > 14 || Number(om) > 59 ||
      (Number(oh) === 14 && Number(om) !== 0))) return false;
  return Number.isFinite(Date.parse(value));
}
function validPolicyConfig(value: unknown): value is PolicyConfig {
  return isRecord(value) && typeof value.enabled === "boolean" &&
    Number.isSafeInteger(value.max_depth) && Number(value.max_depth) >= 1 && Number(value.max_depth) <= 32 &&
    canonicalRatio(value.ratio_cap) && validMode(value.mode) &&
    (value.cycle === "weekly" || value.cycle === "monthly");
}
function validNodeConfig(value: unknown): value is NodeConfig {
  return isRecord(value) && canonicalRatio(value.ratio) &&
    (value.mode === null || validMode(value.mode)) &&
    (value.status === "active" || value.status === "disabled") &&
    typeof value.can_create_children === "boolean";
}
function sameConfig(left: unknown, right: unknown): boolean {
  return isRecord(left) && isRecord(right) &&
    Object.keys(left).length === Object.keys(right).length &&
    Object.keys(right).every((key) => left[key] === right[key]);
}
function validNode(value: unknown, brandId: string, expected?: {
  id?: string;
  memberId?: string;
  parentId?: string | null;
  version?: number;
  policyVersion?: number;
  parentVersion?: number | null;
  config?: NodeConfig;
}): value is AgentNode {
  if (!isRecord(value) || !validUuid(value.id) || (expected?.id !== undefined && value.id !== expected.id) ||
      value.brand_id !== brandId || !validUuid(value.member_id) ||
      (expected?.memberId !== undefined && value.member_id !== expected.memberId) ||
      !(value.parent_id === null || validUuid(value.parent_id)) ||
      (expected && "parentId" in expected && value.parent_id !== expected.parentId) ||
      !positiveInt(value.depth) || value.depth > 32 || !Array.isArray(value.path) || value.path.length === 0 ||
      !value.path.every(validUuid) || value.path[value.path.length - 1] !== value.id ||
      new Set(value.path).size !== value.path.length || value.depth !== value.path.length ||
      (value.parent_id === null ? value.path.length !== 1 : value.path[value.path.length - 2] !== value.parent_id) ||
      !positiveInt(value.version) || (expected?.version !== undefined && value.version !== expected.version) ||
      !validNodeConfig(value.config) || (expected?.config && !sameConfig(value.config, expected.config)) ||
      !validMode(value.effective_mode) ||
      !(value.mode_source_agent_id === null || (validUuid(value.mode_source_agent_id) && value.path.includes(value.mode_source_agent_id))) ||
      (value.config.mode !== null && (value.mode_source_agent_id !== value.id || value.effective_mode !== value.config.mode)) ||
      !positiveInt(value.policy_version) || (expected?.policyVersion !== undefined && value.policy_version !== expected.policyVersion) ||
      !(value.parent_version === null || positiveInt(value.parent_version)) ||
      (expected && "parentVersion" in expected && value.parent_version !== expected.parentVersion) ||
      (value.parent_id === null ? value.parent_version !== null : value.parent_version === null) ||
      !validUuid(value.created_by) || !validDate(value.created_at) || !validDate(value.updated_at) ||
      (value.audit_log_id !== undefined && value.audit_log_id !== null && !validUuid(value.audit_log_id))) return false;
  return true;
}
function validPolicy(value: unknown, brandId: string, expected?: { version: number; config: PolicyConfig }): value is AgentPolicy {
  return isRecord(value) && value.brand_id === brandId && positiveInt(value.version) &&
    (expected === undefined || value.version === expected.version) && validPolicyConfig(value.config) &&
    (expected === undefined || sameConfig(value.config, expected.config)) && validDate(value.updated_at) &&
    (value.audit_log_id === undefined || value.audit_log_id === null || validUuid(value.audit_log_id));
}
function validRevision(value: unknown, brandId: string, agentId: string | null): value is Revision {
  return isRecord(value) && value.brand_id === brandId && value.agent_id === agentId &&
    positiveInt(value.version) && (agentId === null ? validPolicyConfig(value.config) : validNodeConfig(value.config)) &&
    ["system", "admin", "user"].includes(String(value.actor_type)) &&
    (value.actor_id === null || validUuid(value.actor_id)) && typeof value.reason === "string" &&
    validDate(value.created_at) && (value.audit_log_id === null || validUuid(value.audit_log_id));
}
function invalidInput(message: string): never {
  throw new AdminApiError(message, 0, "AGENT_INPUT_INVALID");
}
function invalidResponse(write: boolean): never {
  throw new AdminApiError("Invalid agent API response", write ? 0 : 502, "INVALID_RESPONSE");
}
function pageArgs(limit: number, offset: number): void {
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > MAX_PAGE_SIZE ||
      !Number.isSafeInteger(offset) || offset < 0 || offset > MAX_OFFSET)
    invalidInput("Pagination parameters are out of range");
}
function validPage(value: unknown, brandId: string, agentId: string | null, limit: number, offset: number): value is AgentRevisionPage {
  return isRecord(value) && value.brand_id === brandId && value.agent_id === agentId &&
    Array.isArray(value.items) && value.items.length <= limit &&
    value.items.every((item) => validRevision(item, brandId, agentId)) &&
    value.limit === limit && value.offset === offset && typeof value.total_count === "string" && /^(0|[1-9]\d*)$/.test(value.total_count);
}

export function agentsPermissions(account: AdminAccount, brand: string): {
  policyView: boolean; policyWrite: boolean; view: boolean; write: boolean;
} {
  const scoped = validUuid(account.id) && validUuid(brand) && account.brand_ids.includes(brand);
  const brandPermissions = brandPermissionSet(account, brand);
  return {
    policyView: scoped && brandPermissions.has("agent_policy.view.brand"),
    policyWrite: scoped && !account.super_admin && brandPermissions.has("agent_policy.write.brand"),
    view: scoped && brandPermissions.has("agent.view.brand"),
    write: scoped && !account.super_admin && brandPermissions.has("agent.write.brand"),
  };
}

export function standardAgentApiError(error: unknown, fallback?: string): AdminApiError {
  return standardAdminApiError(error, fallback ?? "Agent request failed");
}

export function createAgentsApi(fetchImpl: typeof fetch = fetch) {
  async function request(path: string, brand: string, options: {
    method?: "GET" | "PUT" | "POST";
    body?: unknown;
    key?: string;
  } = {}): Promise<unknown> {
    if (!validUuid(brand)) invalidInput("Brand ID must be a UUID");
    const write = options.method !== undefined && options.method !== "GET";
    if (write && (typeof options.key !== "string" || options.key.length === 0))
      invalidInput("Idempotency-Key is required for writes");
    const headers = new Headers({ Accept: "application/json", "X-Brand-ID": brand });
    if (options.body !== undefined) headers.set("Content-Type", "application/json");
    if (options.key !== undefined) headers.set("Idempotency-Key", options.key);
    let response: Response;
    try {
      response = await fetchImpl(path, {
        method: options.method ?? "GET",
        credentials: "same-origin",
        headers,
        ...(options.body === undefined ? {} : { body: JSON.stringify(options.body) }),
      });
    } catch (error) {
      throw standardAdminApiError(error);
    }
    let envelope: Envelope;
    try {
      envelope = await response.json() as Envelope;
    } catch {
      throw new AdminApiError(response.ok ? "Invalid server response" : `Request failed (${response.status})`,
        response.ok && write ? 0 : response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : undefined);
    }
    if (!response.ok || envelope.success !== true || envelope.data === undefined) {
      const detail = typeof envelope.error === "object" && envelope.error ? envelope.error : undefined;
      throw new AdminApiError(typeof envelope.error === "string" ? envelope.error : detail?.message ?? `Request failed (${response.status})`,
        response.ok && write ? 0 : response.ok ? 502 : response.status,
        response.ok ? "INVALID_RESPONSE" : detail?.code);
    }
    return envelope.data;
  }

  async function policy(brand: string): Promise<AgentPolicy> {
    const value = await request(`${BASE}/agent-policy`, brand);
    if (!validPolicy(value, brand)) invalidResponse(false);
    return value;
  }
  async function policyHistory(brand: string, limit = 20, offset = 0): Promise<AgentRevisionPage> {
    pageArgs(limit, offset);
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    const value = await request(`${BASE}/agent-policy/history?${query}`, brand);
    if (!validPage(value, brand, null, limit, offset)) invalidResponse(false);
    return value;
  }
  async function tree(brand: string, parentId: string | null = null, limit = 20, offset = 0): Promise<AgentTree> {
    pageArgs(limit, offset);
    if (parentId !== null && !validUuid(parentId)) invalidInput("Parent ID must be a UUID or null");
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (parentId !== null) query.set("parent_id", parentId);
    const value = await request(`${BASE}/agents/tree?${query}`, brand);
    if (!isRecord(value) || value.brand_id !== brand || value.parent_id !== parentId ||
        !Array.isArray(value.items) || value.items.length > limit ||
        !value.items.every((item) => validNode(item, brand, { parentId })) ||
        value.limit !== limit || value.offset !== offset || typeof value.total_count !== "string" || !/^(0|[1-9]\d*)$/.test(value.total_count))
      invalidResponse(false);
    return value as unknown as AgentTree;
  }
  async function node(brand: string, id: string): Promise<AgentNode> {
    if (!validUuid(id)) invalidInput("Agent ID must be a UUID");
    const value = await request(`${BASE}/agents/${encodeURIComponent(id)}`, brand);
    if (!validNode(value, brand, { id })) invalidResponse(false);
    return value;
  }
  async function nodeHistory(brand: string, id: string, limit = 20, offset = 0): Promise<AgentRevisionPage> {
    if (!validUuid(id)) invalidInput("Agent ID must be a UUID");
    pageArgs(limit, offset);
    const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    const value = await request(`${BASE}/agents/${encodeURIComponent(id)}/history?${query}`, brand);
    if (!validPage(value, brand, id, limit, offset)) invalidResponse(false);
    return value;
  }

  return {
    policy,
    async savePolicy(brand: string, body: SaveAgentPolicyBody, key: string): Promise<AgentPolicy> {
      if (!isRecord(body) || !positiveInt(body.version) || body.version >= Number.MAX_SAFE_INTEGER ||
          !validPolicyConfig(body.config) || !validReason(body.reason))
        invalidInput("Agent policy request is invalid");
      const value = await request(`${BASE}/agent-policy`, brand, { method: "PUT", body, key });
      if (!validPolicy(value, brand, { version: body.version + 1, config: body.config })) invalidResponse(true);
      return value;
    },
    policyHistory,
    tree,
    node,
    async createNode(brand: string, body: CreateAgentBody, key: string): Promise<AgentNode> {
      if (!isRecord(body) || !positiveInt(body.policy_version) || !validUuid(body.member_id) ||
          !(body.parent_id === null || validUuid(body.parent_id)) ||
          !(body.parent_version === null || positiveInt(body.parent_version)) ||
          (body.parent_id === null ? body.parent_version !== null : body.parent_version === null) ||
          !validNodeConfig(body.config) || !validReason(body.reason)) invalidInput("Agent creation request is invalid");
      const value = await request(`${BASE}/agents`, brand, { method: "POST", body, key });
      if (!validNode(value, brand, { memberId: body.member_id, parentId: body.parent_id, version: 1, policyVersion: body.policy_version,
        parentVersion: body.parent_version, config: body.config })) invalidResponse(true);
      return value;
    },
    async updateNode(brand: string, id: string, body: UpdateAgentBody, key: string): Promise<AgentNode> {
      if (!validUuid(id)) invalidInput("Agent ID must be a UUID");
      if (!isRecord(body) || !positiveInt(body.version) || body.version >= Number.MAX_SAFE_INTEGER || !positiveInt(body.policy_version) ||
          !(body.parent_version === null || positiveInt(body.parent_version)) ||
          !validNodeConfig(body.config) || !validReason(body.reason)) invalidInput("Agent update request is invalid");
      const value = await request(`${BASE}/agents/${encodeURIComponent(id)}`, brand, { method: "PUT", body, key });
      if (!validNode(value, brand, { id, version: body.version + 1,
        policyVersion: body.policy_version, parentVersion: body.parent_version, config: body.config })) invalidResponse(true);
      return value;
    },
    nodeHistory,
  };
}

function validReason(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0 && new TextEncoder().encode(value).length <= 500;
}

export type AgentsApi = ReturnType<typeof createAgentsApi>;
