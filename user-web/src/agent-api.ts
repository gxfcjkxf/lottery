import { walletBasePath } from "./wallet-api";

export type AgentMode = "loss" | "turnover";
export type AgentStatus = "active" | "disabled";
export interface AgentNodeConfig {
  ratio: string;
  mode: AgentMode | null;
  status: AgentStatus;
  can_create_children: boolean;
}
export interface AgentNode {
  id: string;
  brand_id: string;
  member_id: string;
  parent_id: string | null;
  depth: number;
  path: string[];
  version: number;
  config: AgentNodeConfig;
  effective_mode: AgentMode;
  mode_source_agent_id: string | null;
  policy_version: number;
  parent_version: number | null;
  created_by: string;
  created_at: string;
  updated_at: string;
  audit_log_id?: string;
}
export interface AgentNodePage {
  brand_id: string;
  parent_id: string;
  items: AgentNode[];
  limit: number;
  offset: number;
  total_count: string;
}
export interface AgentChildUpdate {
  version: number;
  policy_version: number;
  parent_version: number;
  ratio: string;
  mode: AgentMode | null;
  reason: string;
}
export interface AgentUpdateScope {
  brandId: string;
  memberId: string;
  parentId: string;
  childMemberId?: string;
}

interface Envelope<T> {
  success?: boolean;
  data?: T;
  error?: string | { code?: string; message?: string } | null;
}

export class AgentApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "AgentApiError";
  }
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const RATIO = /^(?:0|1|(?:0\.[0-9]{0,5}[1-9]))$/;
const KEY = /^[A-Za-z0-9_:.-]{8,128}$/;
const MODES = new Set(["loss", "turnover"]);

function invalid(message: string): never {
  throw new AgentApiError(`Malformed agent API response: ${message}`, 502, "invalid_response");
}
function record(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return invalid(`${label} must be an object`);
  return value as Record<string, unknown>;
}
function id(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) return invalid(`${label} must be a UUID`);
  return value;
}
function positiveInt(value: unknown, label: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 1) return invalid(`${label} must be a positive integer`);
  return value;
}
function ratio(value: unknown, label: string): string {
  if (typeof value !== "string" || !RATIO.test(value)) return invalid(`${label} must be a canonical fraction from 0 to 1 with at most six decimals`);
  return value;
}
function mode(value: unknown, label: string): AgentMode {
  if (typeof value !== "string" || !MODES.has(value)) return invalid(`${label} must be loss or turnover`);
  return value as AgentMode;
}
function parseNode(value: unknown): AgentNode {
  const n = record(value, "node");
  const config = record(n.config, "node.config");
  const nodeId = id(n.id, "node.id");
  const brandId = id(n.brand_id, "node.brand_id");
  const memberId = id(n.member_id, "node.member_id");
  const parentId = n.parent_id === null ? null : id(n.parent_id, "node.parent_id");
  if (!Array.isArray(n.path) || n.path.length === 0 || n.path.length > 32) return invalid("node.path must be a nonempty UUID path");
  const path = n.path.map((part, index) => id(part, `node.path[${index}]`));
  const depth = positiveInt(n.depth, "node.depth");
  if (depth !== path.length || path[path.length - 1] !== nodeId || new Set(path).size !== path.length || (parentId === null ? depth !== 1 : path.length < 2 || path[path.length - 2] !== parentId)) return invalid("node path must be a unique root-to-self path matching its parent and depth");
  const nodeMode = config.mode === null ? null : mode(config.mode, "node.config.mode");
  if (typeof config.status !== "string" || !["active", "disabled"].includes(config.status)) return invalid("node.config.status is unsupported");
  if (typeof config.can_create_children !== "boolean") return invalid("node.config.can_create_children must be boolean");
  if (typeof n.created_at !== "string" || !Number.isFinite(Date.parse(n.created_at)) || typeof n.updated_at !== "string" || !Number.isFinite(Date.parse(n.updated_at))) return invalid("node timestamps must be valid date strings");
  const audit = n.audit_log_id === undefined ? undefined : id(n.audit_log_id, "node.audit_log_id");
  return {
    id: nodeId, brand_id: brandId, member_id: memberId, parent_id: parentId, depth, path,
    version: positiveInt(n.version, "node.version"),
    config: { ratio: ratio(config.ratio, "node.config.ratio"), mode: nodeMode, status: config.status as AgentStatus, can_create_children: config.can_create_children },
    effective_mode: mode(n.effective_mode, "node.effective_mode"),
    mode_source_agent_id: n.mode_source_agent_id === null ? null : id(n.mode_source_agent_id, "node.mode_source_agent_id"),
    policy_version: positiveInt(n.policy_version, "node.policy_version"),
    parent_version: n.parent_version === null ? null : positiveInt(n.parent_version, "node.parent_version"),
    created_by: id(n.created_by, "node.created_by"), created_at: n.created_at, updated_at: n.updated_at,
    ...(audit === undefined ? {} : { audit_log_id: audit }),
  };
}
function inputId(value: unknown, label: string): string {
  if (typeof value !== "string" || !UUID.test(value)) throw new AgentApiError(`${label} must be a UUID`, 0, "invalid_parameter");
  return value;
}
function validateRatioInput(value: unknown): string {
  if (typeof value !== "string" || !RATIO.test(value)) throw new AgentApiError("ratio must be a canonical fraction from 0 to 1 with at most six decimals", 0, "invalid_parameter");
  return value;
}
function validateUpdate(body: AgentChildUpdate): AgentChildUpdate {
  if (!body || typeof body !== "object") throw new AgentApiError("body must be an object", 0, "invalid_parameter");
  if (![body.version, body.policy_version, body.parent_version].every((n) => Number.isSafeInteger(n) && n >= 1)) throw new AgentApiError("versions must be positive integers", 0, "invalid_parameter");
  const cleanRatio = validateRatioInput(body.ratio);
  if (body.mode !== null && !MODES.has(body.mode)) throw new AgentApiError("mode must be loss, turnover or null", 0, "invalid_parameter");
  if (typeof body.reason !== "string" || body.reason.trim().length < 1 || body.reason.length > 500) throw new AgentApiError("reason must contain 1 to 500 characters", 0, "invalid_parameter");
  return { version: body.version, policy_version: body.policy_version, parent_version: body.parent_version, ratio: cleanRatio, mode: body.mode, reason: body.reason };
}

export function createAgentApi(options: { brandCode?: string; fetchImpl?: typeof fetch } = {}) {
  const brandCode = options.brandCode ?? import.meta.env.VITE_BRAND_CODE ?? undefined;
  const fetcher = options.fetchImpl ?? fetch;
  const base = walletBasePath(brandCode);

  async function request(path: string, init: RequestInit = {}): Promise<unknown> {
    const headers = new Headers({ Accept: "application/json" });
    new Headers(init.headers).forEach((value, key) => headers.set(key, value));
    let response: Response;
    try {
      response = await fetcher(`${base}${path}`, { method: "GET", credentials: "include", ...init, headers });
    } catch (cause) {
      throw new AgentApiError(cause instanceof Error ? cause.message : "Network request failed", 0, "network_error");
    }
    let envelope: Envelope<unknown>;
    try { envelope = await response.json() as Envelope<unknown>; }
    catch (cause) {
      if (!response.ok) throw new AgentApiError(`Request failed (${response.status})`, response.status);
      throw new AgentApiError(cause instanceof Error ? cause.message : "Malformed JSON response", 502, "invalid_response");
    }
    if (!response.ok) {
      const detail = typeof envelope?.error === "object" && envelope.error ? envelope.error : undefined;
      throw new AgentApiError(typeof envelope?.error === "string" ? envelope.error : detail?.message ?? `Request failed (${response.status})`, response.status, detail?.code);
    }
    if (!envelope || envelope.success !== true || envelope.data === undefined) return invalid("successful HTTP response must contain a success envelope");
    return envelope.data;
  }

  return {
    async me(): Promise<AgentNode> {
      return parseNode(await request("/agent/me"));
    },
    async children(limit = 20, offset = 0): Promise<AgentNodePage> {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1_000_000) throw new AgentApiError("limit must be 1 to 100 and offset 0 to 1000000", 0, "invalid_parameter");
      const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      const data = record(await request(`/agent/children?${query}`), "data");
      const brandId = id(data.brand_id, "brand_id");
      const parentId = id(data.parent_id, "parent_id");
      if (!Array.isArray(data.items) || data.items.length > limit) return invalid("items must be an array within the requested limit");
      const items = data.items.map(parseNode);
      if (items.some((item) => item.brand_id !== brandId || item.parent_id !== parentId) || new Set(items.map((item) => item.id)).size !== items.length) return invalid("children must be unique direct children in the page scope");
      if (data.limit !== limit || data.offset !== offset || typeof data.total_count !== "string" || !/^(?:0|[1-9]\d*)$/.test(data.total_count)) return invalid("pagination fields must match the request and use canonical total_count");
      return { brand_id: brandId, parent_id: parentId, items, limit, offset, total_count: data.total_count };
    },
    async updateChild(childId: string, input: AgentChildUpdate, idempotencyKey: string, expected?: AgentUpdateScope): Promise<AgentNode> {
      const target = inputId(childId, "childId");
      const body = validateUpdate(input);
      if (typeof idempotencyKey !== "string" || !KEY.test(idempotencyKey)) throw new AgentApiError("idempotencyKey must contain 8 to 128 allowed characters", 0, "invalid_parameter");
      let scope: AgentUpdateScope | undefined;
      if (expected) scope = {
        brandId: inputId(expected.brandId, "expected.brandId"),
        memberId: inputId(expected.memberId, "expected.memberId"),
        parentId: inputId(expected.parentId, "expected.parentId"),
        ...(expected.childMemberId === undefined ? {} : { childMemberId: inputId(expected.childMemberId, "expected.childMemberId") }),
      };
      const data = await request(`/agent/children/${encodeURIComponent(target)}/config`, { method: "PUT", headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(body) });
      let node: AgentNode;
      try { node = parseNode(data); }
      catch (cause) { throw cause; }
      const mismatch = node.id !== target || (scope && (
        node.brand_id !== scope.brandId ||
        node.parent_id !== scope.parentId ||
        (scope.childMemberId !== undefined && node.member_id !== scope.childMemberId)
      )) || node.version !== body.version + 1 || node.policy_version !== body.policy_version || node.parent_version !== body.parent_version || node.config.ratio !== body.ratio || node.config.mode !== body.mode;
      if (mismatch) throw new AgentApiError("Update receipt does not match the requested child configuration and versions", 502, "invalid_receipt");
      return node;
    },
  };
}

export type AgentApi = ReturnType<typeof createAgentApi>;
