import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as RewardsApi from "./rewards-api";
import * as RewardsState from "./rewards-state";
import type { RewardAction, RewardGrantBody, RewardOrder } from "./rewards-api";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "33333333-3333-4333-8333-333333333333";
const member = "55555555-5555-4555-8555-555555555555";
const orderID = "44444444-4444-4444-8444-444444444444";
const ledger = "77777777-7777-4777-8777-777777777777";
const audit = "88888888-8888-4888-8888-888888888888";
const actionID = "99999999-9999-4999-8999-999999999999";
const timestamp = "2026-10-08T10:00:00Z";
const requestID = "rewards-component-request-001";
const allPermissions = ["reward.view.brand", "reward.grant.brand", "reward.revoke.brand", "reward.retry.brand"];
const baseAccount: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: allPermissions },
};

const RewardsComponent = compileComponent();
type HostNode = {
  tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode;
  value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void;
  removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[];
  listeners: Record<string, Array<(event: { target: HostNode }) => void>>;
};
function element(tag: string): HostNode {
  const node = { tag, props: {}, children: [] as HostNode[], text: "", value: "", checked: false, listeners: {} as HostNode["listeners"],
    addEventListener(type: string, listener: (event: { target: HostNode }) => void) { (this.listeners[type] ??= []).push(listener); },
    removeEventListener(type: string, listener: (event: { target: HostNode }) => void) { this.listeners[type] = (this.listeners[type] ?? []).filter((candidate) => candidate !== listener); },
    getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } };
  return node as HostNode;
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element,
  createText: (text) => ({ ...element("#text"), text }),
  createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; },
  setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => { if (node.parent) { const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); } node.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(node); else parent.children.splice(i, 0, node); },
  remove: (node) => { if (!node.parent) return; const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null,
  nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});

function compileComponent(): Component {
  const source = readFileSync(new URL("./RewardsManagement.vue", import.meta.url), "utf8");
  const descriptor = parseVue(source, { filename: "RewardsManagement.vue" }).descriptor;
  const compiled = compileVueScript(descriptor, { id: "rewards-component-test", inlineTemplate: true }).content;
  const javascript = transpileScript(compiled);
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./rewards-api": RewardsApi, "./rewards-state": RewardsState,
  };
  const body = javascript
    .replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) => {
      if (!(specifier in modules)) throw new Error(`Unmapped test import: ${specifier}`);
      return `const {${bindings.replace(/\s+as\s+/g, ": ")}} = __modules[${JSON.stringify(specifier)}];`;
    })
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, binding: string, specifier: string) => {
      if (!(specifier in modules)) throw new Error(`Unmapped test import: ${specifier}`);
      return `const ${binding} = __modules[${JSON.stringify(specifier)}].default;`;
    })
    .replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
import { compileScript as compileVueScript, parse as parseVue } from "vue/compiler-sfc";
import ts from "typescript";
function transpileScript(source: string): string { return ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText; }

function reward(overrides: Partial<RewardOrder> = {}): RewardOrder {
  return { id: orderID, brand_id: brand, member_id: member, points: "25", state: "granted", version: 1,
    grant_ledger_entry_id: ledger, revoke_ledger_entry_id: null, creation_audit_log_id: audit,
    last_audit_log_id: audit, last_error_code: null, created_by: actor, reason: "Welcome reward",
    point_policy_version: "1", created_at: timestamp, updated_at: timestamp, revoked_at: null, ...overrides };
}
function action(overrides: Partial<RewardAction> = {}): RewardAction {
  return { id: actionID, brand_id: brand, order_id: orderID, version: 1, operation: "grant", state_before: null,
    state_after: "granted", actor_id: actor, reason: "Welcome reward", audit_log_id: audit,
    ledger_entry_id: ledger, created_at: timestamp, ...overrides };
}
function ok(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data, request_id: requestID }), { status }); }
function failure(status: number, code = "CONFLICT") {
  return new Response(JSON.stringify({ success: false, error: { code, message: "Current reward state changed" }, request_id: requestID }), { status });
}
function deferred<T>() {
  let resolve!: (value: T) => void, reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, match: (candidate: HostNode) => boolean): HostNode | null { if (match(node)) return node; for (const child of node.children) { const found = findNode(child, match); if (found) return found; } return null; }
function byTestId(root: HostNode, id: string): HostNode | null { return findNode(root, (node) => node.props["data-testid"] === id); }
function button(root: HostNode, label: string): HostNode | null { return findNode(root, (node) => node.tag === "button" && textOf(node).includes(label)); }
function click(node: HostNode | null) { expect(node).not.toBeNull(); if (node!.props.disabled) return; (node!.props.onClick as (() => void) | undefined)?.(); }
function setModel(node: HostNode | null, value: unknown) {
  expect(node).not.toBeNull();
  const update = node!.props["onUpdate:modelValue"] as ((next: unknown) => void) | undefined;
  if (update) { update(value); return; }
  if (node!.tag === "input" && node!.props.type === "checkbox") { node!.checked = Boolean(value); for (const listener of node!.listeners.change ?? []) listener({ target: node! }); return; }
  node!.value = String(value ?? "");
  for (const listener of node!.listeners.input ?? []) listener({ target: node! });
}
async function flush() { for (let i = 0; i < 14; i++) await Promise.resolve(); await nextTick(); }
function mount(account: AdminAccount, brandId = brand, brandStatus = "active") {
  const scope = ref({ account, brandId, brandStatus }), container = element("root"), emitted: string[] = [], i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(RewardsComponent, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(container);
  return { app, container, scope, emitted, i18n };
}
function installFetch(options: {
  current?: RewardOrder; orders?: RewardOrder[];
  onWrite?: (call: { url: string; init?: RequestInit; body: unknown }) => Promise<Response> | Response;
  delayList?: ReturnType<typeof deferred<Response>>; failDetailOnce?: boolean; failDetailAfterWrite?: boolean;
} = {}) {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  let current = options.current ?? reward(), writeAccepted = false, failedDetailReads = 0;
  const calls: Array<{ url: string; init?: RequestInit; body?: unknown }> = [];
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input), method = init?.method ?? "GET";
    const body = init?.body === undefined ? undefined : JSON.parse(String(init.body));
    calls.push({ url, init, body });
    if (method === "GET" && url.includes("/actions?")) return ok({ brand_id: brand, order_id: orderID, items: [action()], total_count: "1", limit: 20, offset: 0 });
    if (method === "GET" && url.endsWith(`/${orderID}`)) {
      if (options.failDetailOnce || (options.failDetailAfterWrite && writeAccepted)) { options.failDetailOnce = false; options.failDetailAfterWrite = false; failedDetailReads++; return failure(503, "REWARD_BUSY"); }
      return ok(current);
    }
    if (method === "GET" && url.includes("/reward-orders?")) {
      if (options.delayList) return options.delayList.promise;
      const items = options.orders ?? [current];
      return ok({ brand_id: brand, items, total_count: String(items.length), limit: 20, offset: 0 });
    }
    if (method === "POST") {
      const custom = options.onWrite?.({ url, init, body });
      if (custom) { const response = await custom; if (response.ok) writeAccepted = true; return response; }
      if (url === "/api/v1/admin/reward-orders") {
        const grantBody = body as { member_id: string; points: string; reason: string };
        current = reward({ member_id: grantBody.member_id, points: grantBody.points, reason: grantBody.reason });
        writeAccepted = true;
        return ok(current, 201);
      }
      const actionBody = body as { version: number; reason: string };
      if (url.endsWith("/revoke")) {
        current = reward({ state: "revocation_pending", version: actionBody.version + 1, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT", reason: actionBody.reason });
        writeAccepted = true;
        return ok(current);
      }
      current = reward({ state: "revoked", version: actionBody.version + 1, revoke_ledger_entry_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", revoked_at: timestamp, reason: actionBody.reason });
      writeAccepted = true;
      return ok(current);
    }
    return failure(404, "REQUEST_INVALID");
  });
  vi.stubGlobal("fetch", fetcher);
  return { calls, fetcher, get current() { return current; }, set current(value: RewardOrder) { current = value; }, get failedDetailReads() { return failedDetailReads; } };
}

function writes(server: ReturnType<typeof installFetch>) { return server.calls.filter((call) => call.init?.method === "POST"); }
async function fillGrant(mounted: ReturnType<typeof mount>, reason: string, points = "25") {
  setModel(byTestId(mounted.container, "rewards-grant-member"), member);
  setModel(byTestId(mounted.container, "rewards-grant-points"), points);
  setModel(byTestId(mounted.container, "rewards-grant-reason"), reason);
  await flush();
}
async function confirmReview(mounted: ReturnType<typeof mount>) {
  expect(byTestId(mounted.container, "rewards-confirm-submit")?.props.disabled).toBe(true);
  setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush();
  expect(byTestId(mounted.container, "rewards-confirm-submit")?.props.disabled).toBe(false);
  click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
}
async function submitGrant(mounted: ReturnType<typeof mount>, reason: string, points = "25") {
  await fillGrant(mounted, reason, points);
  expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(false);
  click(byTestId(mounted.container, "rewards-grant-review")); await flush();
  await confirmReview(mounted);
}
async function submitRevoke(mounted: ReturnType<typeof mount>, reason: string) {
  click(button(mounted.container, orderID)); await flush();
  setModel(byTestId(mounted.container, "rewards-action-reason"), reason); await flush();
  expect(byTestId(mounted.container, "rewards-revoke")?.props.disabled).toBe(false);
  click(byTestId(mounted.container, "rewards-revoke")); await flush();
  await confirmReview(mounted);
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); RewardsState.clearAllRewardIntents(); });

describe("RewardsManagement", () => {
  it("renders the required stable root class and key controls", async () => {
    installFetch(); const mounted = mount(baseAccount); await flush();
    const root = findNode(mounted.container, (node) => node.tag === "article");
    expect(root?.props.class).toContain("rewards-management");
    for (const id of ["rewards-grant-member", "rewards-grant-points", "rewards-grant-reason", "rewards-grant-review", "rewards-list-refresh"]) {
      expect(byTestId(mounted.container, id)).not.toBeNull();
    }
    mounted.app.unmount();
  });

  it("requires brand-scoped view permission and never inherits platform or super-admin privileges", async () => {
    for (const account of [
      { ...baseAccount, platform_permissions: [], permissions_by_brand: { [brand]: ["reward.grant.brand"] }, permissions: ["reward.view.platform"] },
      { ...baseAccount, super_admin: true, permissions_by_brand: { [brand]: [] } },
    ]) {
      const server = installFetch(); const mounted = mount(account); await flush();
      expect(server.calls).toHaveLength(0);
      expect(byTestId(mounted.container, "rewards-list-refresh")).toBeNull();
      mounted.app.unmount();
    }
    const readOnly = mount({ ...baseAccount, platform_permissions: [], permissions_by_brand: { [brand]: ["reward.view.brand"] }, permissions: ["reward.grant.brand"] }); await flush();
    expect(readOnly.container.children.length).toBeGreaterThan(0);
    expect(byTestId(readOnly.container, "rewards-grant-review")?.props.disabled).toBe(true);
    readOnly.app.unmount();
  });

  it("allows only the explicitly granted operation for the selected brand", async () => {
    const server = installFetch({ current: reward({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" }) });
    const mounted = mount({ ...baseAccount, permissions_by_brand: { [brand]: ["reward.view.brand", "reward.retry.brand"] } }); await flush();
    click(button(mounted.container, orderID)); await flush();
    expect(byTestId(mounted.container, "rewards-retry")).not.toBeNull();
    expect(byTestId(mounted.container, "rewards-revoke")).toBeNull();
    expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(0);
    mounted.app.unmount();
  });

  it("reviews a grant with member, points, reason, actor, key and frozen body before explicit confirmation", async () => {
    let server!: ReturnType<typeof installFetch>;
    server = installFetch({ onWrite: ({ body }) => ok(reward({ member_id: (body as { member_id: string }).member_id, points: (body as { points: string }).points }), 201) });
    const mounted = mount(baseAccount); await flush();
    setModel(byTestId(mounted.container, "rewards-grant-member"), member); await flush();
    setModel(byTestId(mounted.container, "rewards-grant-points"), "9223372036854775807"); await flush();
    setModel(byTestId(mounted.container, "rewards-grant-reason"), "Approved customer reward"); await flush();
    click(byTestId(mounted.container, "rewards-grant-review")); await flush();
    expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(0);
    expect(textOf(byTestId(mounted.container, "rewards-review")!)).toContain("Approved customer reward");
    expect(textOf(byTestId(mounted.container, "rewards-review")!)).toContain(member);
    setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    const writes = server.calls.filter((call) => call.init?.method === "POST");
    expect(writes).toHaveLength(1);
    expect(writes[0].url).toBe("/api/v1/admin/reward-orders");
    expect(writes[0].body).toEqual({ member_id: member, points: "9223372036854775807", reason: "Approved customer reward" });
    expect(new Headers(writes[0].init?.headers).get("X-Reward-Actor-ID")).toBe(actor);
    expect(new Headers(writes[0].init?.headers).get("Idempotency-Key")).toBeTruthy();
    expect(byTestId(mounted.container, "rewards-receipt")).not.toBeNull();
    mounted.app.unmount();
  });

  it("reviews revoke and retry according to current order eligibility", async () => {
    for (const [snapshot, operation, testId, route] of [
      [reward(), "revoke", "rewards-revoke", "/revoke"],
      [reward({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" }), "retry", "rewards-retry", "/retry-revocation"],
    ] as const) {
      const server = installFetch({ current: snapshot }); const mounted = mount(baseAccount); await flush();
      click(button(mounted.container, orderID)); await flush();
      setModel(byTestId(mounted.container, "rewards-action-reason"), `Approved ${operation}`); await flush();
      click(byTestId(mounted.container, testId)); await flush();
      setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush();
      click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
      const write = server.calls.find((call) => call.init?.method === "POST");
      expect(write?.url).toContain(`/${orderID}${route}`);
      expect(write?.body).toEqual({ version: snapshot.version, reason: `Approved ${operation}` });
      expect(mounted.app).toBeDefined(); mounted.app.unmount();
    }
    for (const snapshot of [reward({ state: "revoked", version: 2, revoke_ledger_entry_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", revoked_at: timestamp })]) {
      const server = installFetch({ current: snapshot }); const mounted = mount(baseAccount); await flush(); click(button(mounted.container, orderID)); await flush();
      expect(byTestId(mounted.container, "rewards-revoke")).toBeNull();
      expect(byTestId(mounted.container, "rewards-retry")).toBeNull();
      expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(0); mounted.app.unmount();
    }
  });

  it("retains an unknown grant unchanged across unmount and remount, and retries it only after renewed review", async () => {
    const requests: Array<{ url: string; init?: RequestInit; body: unknown }> = [];
    let attempt = 0;
    const server = installFetch({ onWrite: (call) => { requests.push({ ...call, body: call.body }); attempt++; if (attempt === 1) throw new Error("response lost after server accepted write"); return ok(reward({ member_id: (call.body as { member_id: string }).member_id, points: (call.body as { points: string }).points }), 201); } });
    let mounted = mount(baseAccount); await flush();
    setModel(byTestId(mounted.container, "rewards-grant-member"), member); setModel(byTestId(mounted.container, "rewards-grant-points"), "25"); setModel(byTestId(mounted.container, "rewards-grant-reason"), "Keep original request"); await flush();
    click(byTestId(mounted.container, "rewards-grant-review")); await flush(); setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    expect(textOf(mounted.container)).toContain("结果未知"); mounted.app.unmount();

    server.current = reward({ state: "granted", version: 1, reason: "Keep original request" });
    mounted = mount(baseAccount); await flush();
    expect(textOf(mounted.container)).toContain("待确认请求");
    const frozenBefore = RewardsState.getPendingRewardIntent({ accountId: actor, brandId: brand, operation: "grant" });
    expect(frozenBefore?.body).toEqual({ member_id: member, points: "25", reason: "Keep original request" });
    click(byTestId(mounted.container, "rewards-list-refresh")); await flush();
    expect(RewardsState.getPendingRewardIntent({ accountId: actor, brandId: brand, operation: "grant" })).toEqual(frozenBefore);
    expect(button(mounted.container, "使用原请求重试")).not.toBeNull();
    click(button(mounted.container, "使用原请求重试")); await flush();
    expect(textOf(byTestId(mounted.container, "rewards-review")!)).toContain("Keep original request");
    setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush(); click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    expect(requests).toHaveLength(2);
    expect(requests[1].body).toEqual(requests[0].body);
    expect(new Headers(requests[1].init?.headers).get("Idempotency-Key")).toBe(new Headers(requests[0].init?.headers).get("Idempotency-Key"));
    expect(new Headers(requests[1].init?.headers).get("X-Reward-Actor-ID")).toBe(actor);
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(0);
    expect(byTestId(mounted.container, "rewards-receipt")).not.toBeNull();
    mounted.app.unmount();
  });

  it("keeps unknown intent locked even when fresh list, detail and history reads show a later pending state", async () => {
    const server = installFetch({ onWrite: () => { throw new Error("response lost after write"); } });
    const mounted = mount(baseAccount); await flush(); click(button(mounted.container, orderID)); await flush();
    setModel(byTestId(mounted.container, "rewards-action-reason"), "Retry uncertain action"); await flush();
    click(byTestId(mounted.container, "rewards-revoke")); await flush(); setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    const pending = RewardsState.listPendingRewardIntents(actor, brand);
    expect(pending).toHaveLength(1);
    server.current = reward({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" });
    click(byTestId(mounted.container, "rewards-list-refresh")); click(byTestId(mounted.container, "rewards-history-refresh")); click(byTestId(mounted.container, "rewards-detail-refresh")); await flush();
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toEqual(pending);
    expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(1);
    expect(byTestId(mounted.container, "rewards-revoke")).toBeNull();
    expect(byTestId(mounted.container, "rewards-retry")?.props.disabled).toBe(true);
    await fillGrant(mounted, "Another reward requires resolution first");
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    mounted.app.unmount();
  });

  it("preserves a confirmed receipt after a failed detail GET, allows a new grant review and only retries queries", async () => {
    const committed = installFetch({ failDetailAfterWrite: true });
    const second = mount(baseAccount); await flush(); click(button(second.container, orderID)); await flush();
    setModel(byTestId(second.container, "rewards-action-reason"), "Confirmed write, detail unavailable"); await flush();
    click(byTestId(second.container, "rewards-revoke")); await flush(); setModel(byTestId(second.container, "rewards-review-confirmed"), true); await flush();
    click(byTestId(second.container, "rewards-confirm-submit")); await flush();
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(0);
    expect(committed.calls.filter((call) => call.init?.method === "POST")).toHaveLength(1);
    expect(byTestId(second.container, "rewards-receipt")).not.toBeNull();
    expect(textOf(byTestId(second.container, "rewards-receipt")!)).toContain("撤销待处理 · v2");
    expect(button(second.container, "使用原请求重试")).toBeNull();
    expect(committed.failedDetailReads).toBe(1);
    expect(textOf(second.container)).not.toContain("结果未知");
    expect(byTestId(second.container, "rewards-revoke")).toBeNull();
    expect(byTestId(second.container, "rewards-retry")).toBeNull();
    expect(byTestId(second.container, "rewards-action-reason")).toBeNull();
    await fillGrant(second, "A separately reviewed grant after confirmed reversal");
    expect(byTestId(second.container, "rewards-grant-review")?.props.disabled).toBe(false);
    click(byTestId(second.container, "rewards-grant-review")); await flush();
    expect(byTestId(second.container, "rewards-review")).not.toBeNull();
    expect(writes(committed)).toHaveLength(1);
    click(button(second.container, "取消")); await flush();
    const detailReadsBefore = committed.calls.filter((call) => call.url.endsWith(`/${orderID}`) && call.init?.method === "GET").length;
    click(button(second.container, orderID)); await flush();
    expect(committed.calls.filter((call) => call.url.endsWith(`/${orderID}`) && call.init?.method === "GET")).toHaveLength(detailReadsBefore + 1);
    expect(byTestId(second.container, "rewards-retry")).not.toBeNull();
    expect(writes(committed)).toHaveLength(1);
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(0);
    expect(textOf(byTestId(second.container, "rewards-receipt")!)).toContain("撤销待处理 · v2");
    second.app.unmount();
  });

  it("retains a late HTTP 409 after unmount as a conflict requiring fresh reads and explicit discard", async () => {
    const response = deferred<Response>();
    const server = installFetch({ onWrite: () => response.promise });
    let mounted = mount(baseAccount); await flush();
    await submitRevoke(mounted, "Original revoke awaiting a deferred conflict");
    const scope = { accountId: actor, brandId: brand, operation: "revoke" as const, targetId: orderID };
    const original = RewardsState.getPendingRewardIntent(scope)!;
    expect(original).not.toBeNull();
    expect(writes(server)).toHaveLength(1);
    mounted.app.unmount();

    response.resolve(failure(409, "REWARD_VERSION_CONFLICT")); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    expect(RewardsState.isRewardIntentConflict(original)).toBe(true);
    server.current = reward({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" });
    mounted = mount(baseAccount); await flush();
    expect(byTestId(mounted.container, "rewards-conflict-refresh")).not.toBeNull();
    expect(byTestId(mounted.container, "rewards-conflict-discard")?.props.disabled).toBe(true);
    expect(byTestId(mounted.container, "rewards-discard-reviewed")?.props.disabled).toBe(true);
    expect(button(mounted.container, "使用原请求重试")).toBeNull();
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    const readCounts = [
      "/api/v1/admin/reward-orders?limit=20&offset=0",
      `/api/v1/admin/reward-orders/${orderID}`,
      `/api/v1/admin/reward-orders/${orderID}/actions?limit=20&offset=0`,
    ].map((url) => ({ url, count: server.calls.filter((call) => call.url === url && call.init?.method === "GET").length }));
    click(byTestId(mounted.container, "rewards-conflict-discard")); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    click(byTestId(mounted.container, "rewards-conflict-refresh")); await flush();
    for (const { url, count } of readCounts) {
      expect(server.calls.filter((call) => call.url === url && call.init?.method === "GET")).toHaveLength(count + 1);
    }
    expect(byTestId(mounted.container, "rewards-discard-reviewed")?.props.disabled).toBe(false);
    expect(byTestId(mounted.container, "rewards-conflict-discard")?.props.disabled).toBe(true);
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    expect(writes(server)).toHaveLength(1);
    setModel(byTestId(mounted.container, "rewards-discard-reviewed"), true); await flush();
    click(byTestId(mounted.container, "rewards-conflict-discard")); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toBeNull();
    expect(RewardsState.isRewardIntentConflict(original)).toBe(false);
    setModel(byTestId(mounted.container, "rewards-action-reason"), "Review the fresh pending version"); await flush();
    click(byTestId(mounted.container, "rewards-retry")); await flush();
    const review = byTestId(mounted.container, "rewards-review");
    expect(review).not.toBeNull();
    expect(textOf(review!)).toContain('"version": 2');
    expect(textOf(review!)).not.toContain(original.key);
    expect(writes(server)).toHaveLength(1);
    mounted.app.unmount();
  });

  it("retains an exact late unknown grant after unmount and only replays its frozen request after renewed confirmation", async () => {
    const response = deferred<Response>();
    let attempts = 0;
    const server = installFetch({ onWrite: ({ body }) => {
      if (++attempts === 1) return response.promise;
      return ok(reward(body as RewardGrantBody), 201);
    } });
    let mounted = mount(baseAccount); await flush();
    await submitGrant(mounted, "Exact deferred reward request ✅", "9223372036854775807");
    const scope = { accountId: actor, brandId: brand, operation: "grant" as const };
    const original = RewardsState.getPendingRewardIntent(scope)!;
    expect(original).toMatchObject({ ...scope, body: { member_id: member, points: "9223372036854775807", reason: "Exact deferred reward request ✅" } });
    expect(original.targetId).toBeUndefined();
    expect(Object.isFrozen(original.body)).toBe(true);
    expect(writes(server)).toHaveLength(1);
    mounted.app.unmount();
    response.reject(new Error("Network outcome became unknown after unmount")); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    expect(RewardsState.isRewardIntentConflict(original)).toBe(false);
    server.current = reward({ state: "revoked", version: 3, revoke_ledger_entry_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", revoked_at: timestamp });
    mounted = mount(baseAccount); await flush();
    click(button(mounted.container, orderID)); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toEqual(original);
    await fillGrant(mounted, "A new grant remains locked");
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    expect(writes(server)).toHaveLength(1);
    click(button(mounted.container, "使用原请求重试")); await flush();
    const review = byTestId(mounted.container, "rewards-review")!;
    for (const value of [actor, brand, member, original.key, original.body.reason, "9223372036854775807"]) expect(textOf(review)).toContain(value);
    expect(writes(server)).toHaveLength(1);
    await confirmReview(mounted);
    expect(writes(server)).toHaveLength(2);
    const [first, replay] = writes(server);
    expect(replay.url).toBe(first.url);
    expect(replay.init?.body).toBe(first.init?.body);
    for (const header of ["X-Reward-Actor-ID", "X-Brand-ID", "Idempotency-Key"]) {
      expect(new Headers(replay.init?.headers).get(header)).toBe(new Headers(first.init?.headers).get(header));
    }
    expect(RewardsState.getPendingRewardIntent(scope)).toBeNull();
    expect(textOf(byTestId(mounted.container, "rewards-receipt")!)).toContain("已发放 · v1");
    expect(textOf(mounted.container)).toContain("已撤销 · v3");
    mounted.app.unmount();
  });

  it("does not recreate a logged-out intent when its deferred POST later becomes unknown", async () => {
    const response = deferred<Response>(), server = installFetch({ onWrite: () => response.promise });
    let mounted = mount(baseAccount); await flush();
    await submitGrant(mounted, "Logged-out grant must remain cleared");
    const scope = { accountId: actor, brandId: brand, operation: "grant" as const };
    expect(RewardsState.getPendingRewardIntent(scope)).not.toBeNull();
    const generation = RewardsState.rewardSessionGeneration();
    RewardsState.clearAllRewardIntents();
    expect(RewardsState.rewardSessionGeneration()).toBeGreaterThan(generation);
    mounted.app.unmount();
    response.reject(new Error("Late network loss after logout")); await flush();
    expect(RewardsState.getPendingRewardIntent(scope)).toBeNull();
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toEqual([]);
    mounted = mount(baseAccount); await flush();
    expect(button(mounted.container, "使用原请求重试")).toBeNull();
    expect(byTestId(mounted.container, "rewards-conflict-discard")).toBeNull();
    await fillGrant(mounted, "A fresh session can review a fresh request");
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(false);
    expect(writes(server)).toHaveLength(1);
    mounted.app.unmount();
  });

  it("does not let a stale ACK for the previous account unlock the current account's unknown request", async () => {
    const otherActor = "66666666-6666-4666-8666-666666666666";
    const oldResponse = deferred<Response>(), currentResponse = deferred<Response>();
    const server = installFetch({ onWrite: ({ init }) => new Headers(init?.headers).get("X-Reward-Actor-ID") === actor ? oldResponse.promise : currentResponse.promise });
    const mounted = mount(baseAccount); await flush();
    await submitGrant(mounted, "Previous account's deferred grant");
    const oldScope = { accountId: actor, brandId: brand, operation: "grant" as const };
    expect(RewardsState.getPendingRewardIntent(oldScope)).not.toBeNull();
    mounted.scope.value = { ...mounted.scope.value, account: { ...baseAccount, id: otherActor } }; await flush();
    expect(RewardsState.getPendingRewardIntent(oldScope)).toBeNull();
    await submitGrant(mounted, "Current account's frozen unknown grant", "31");
    currentResponse.reject(new Error("Current account's write outcome is unknown")); await flush();
    const currentScope = { accountId: otherActor, brandId: brand, operation: "grant" as const };
    const currentIntent = RewardsState.getPendingRewardIntent(currentScope)!;
    expect(currentIntent).not.toBeNull();
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    oldResponse.resolve(ok(reward({ reason: "Previous account's deferred grant" }), 201)); await flush();
    expect(RewardsState.getPendingRewardIntent(currentScope)).toEqual(currentIntent);
    expect(RewardsState.getPendingRewardIntent(oldScope)).toBeNull();
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    expect(byTestId(mounted.container, "rewards-receipt")).toBeNull();
    click(button(mounted.container, "使用原请求重试")); await flush();
    const review = byTestId(mounted.container, "rewards-review")!;
    for (const value of [otherActor, currentIntent.key, currentIntent.body.reason]) expect(textOf(review)).toContain(value);
    expect(writes(server)).toHaveLength(2);
    expect(new Headers(writes(server)[1].init?.headers).get("X-Reward-Actor-ID")).toBe(otherActor);
    mounted.app.unmount();
  });

  it("does not let a stale ACK after permission loss unlock a different current unresolved intent", async () => {
    const response = deferred<Response>(), server = installFetch({ onWrite: () => response.promise });
    const mounted = mount(baseAccount); await flush();
    await submitRevoke(mounted, "Revoke permission will be removed before ACK");
    const oldScope = { accountId: actor, brandId: brand, operation: "revoke" as const, targetId: orderID };
    const original = RewardsState.getPendingRewardIntent(oldScope)!;
    const currentScope = { accountId: actor, brandId: brand, operation: "grant" as const };
    const currentIntent = RewardsState.createRewardIntent(currentScope, { member_id: member, points: "37", reason: "A separate unresolved grant in the current scope" }, "reward-current-grant-key-001")!;
    RewardsState.setPendingRewardIntent(currentScope, currentIntent);
    mounted.scope.value = { ...mounted.scope.value, account: { ...baseAccount, permissions_by_brand: { [brand]: ["reward.view.brand", "reward.grant.brand"] } } }; await flush();
    expect(textOf(mounted.container)).not.toContain(original.body.reason);
    await fillGrant(mounted, "New grant must wait for the current unknown request");
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    response.resolve(ok(reward({ state: "revocation_pending", version: 2, last_error_code: "REWARD_AVAILABLE_INSUFFICIENT" }))); await flush();
    expect(RewardsState.getPendingRewardIntent(oldScope)).toBeNull();
    expect(RewardsState.getPendingRewardIntent(currentScope)).toEqual(currentIntent);
    expect(byTestId(mounted.container, "rewards-grant-review")?.props.disabled).toBe(true);
    expect(byTestId(mounted.container, "rewards-receipt")).toBeNull();
    click(button(mounted.container, "使用原请求重试")); await flush();
    const review = byTestId(mounted.container, "rewards-review")!;
    expect(textOf(review)).toContain(currentIntent.key);
    expect(textOf(review)).toContain(currentIntent.body.reason);
    expect(textOf(review)).not.toContain(original.key);
    expect(writes(server)).toHaveLength(1);
    mounted.app.unmount();
  });

  it("refreshes related snapshots on 409 and requires an explicit discard checkbox before clearing the conflict", async () => {
    const server = installFetch({ onWrite: () => failure(409) }); const mounted = mount(baseAccount); await flush(); click(button(mounted.container, orderID)); await flush();
    setModel(byTestId(mounted.container, "rewards-action-reason"), "Review stale order"); await flush(); click(byTestId(mounted.container, "rewards-revoke")); await flush();
    setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush(); click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(1);
    expect(RewardsState.listPendingRewardIntents(actor, brand).some((intent) => RewardsState.isRewardIntentConflict(intent))).toBe(true);
    expect(server.calls.filter((call) => call.init?.method === "GET").length).toBeGreaterThan(2);
    expect(byTestId(mounted.container, "rewards-conflict-refresh")).not.toBeNull();
    expect(byTestId(mounted.container, "rewards-conflict-discard")?.props.disabled).toBe(true);
    click(byTestId(mounted.container, "rewards-conflict-refresh")); await flush();
    expect(server.calls.filter((call) => call.init?.method === "GET").length).toBeGreaterThan(4);
    setModel(byTestId(mounted.container, "rewards-discard-reviewed"), true); await flush();
    click(byTestId(mounted.container, "rewards-conflict-discard")); await flush();
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(0);
    mounted.app.unmount();
  });

  it("clears old scope and intent on 401, emits session-invalid, and rejects stale reads after account switch", async () => {
    let mounted = mount(baseAccount); await flush();
    const oldScope = { accountId: "66666666-6666-4666-8666-666666666666", brandId: brand, operation: "grant" as const };
    const intent = RewardsState.createRewardIntent(oldScope, { member_id: member, points: "5", reason: "Pending grant" }, "reward-intent-key-401");
    RewardsState.setPendingRewardIntent(oldScope, intent!);
    mounted.app.unmount();

    const authServer = installFetch({ onWrite: () => failure(401, "AUTH_UNAUTHENTICATED") });
    mounted = mount(baseAccount); await flush(); setModel(byTestId(mounted.container, "rewards-grant-member"), member); setModel(byTestId(mounted.container, "rewards-grant-points"), "5"); setModel(byTestId(mounted.container, "rewards-grant-reason"), "New grant"); await flush();
    click(byTestId(mounted.container, "rewards-grant-review")); await flush(); setModel(byTestId(mounted.container, "rewards-review-confirmed"), true); await flush(); click(byTestId(mounted.container, "rewards-confirm-submit")); await flush();
    expect(mounted.emitted).toContain("session-invalid");
    expect(RewardsState.listPendingRewardIntents(actor, brand)).toHaveLength(0);
    expect(authServer.calls.some((call) => call.init?.method === "POST")).toBe(true);
    mounted.app.unmount();

    const slow = deferred<Response>(); const server = installFetch({ delayList: slow });
    mounted = mount(baseAccount); await flush();
    mounted.scope.value = { ...mounted.scope.value, account: { ...baseAccount, id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", brand_ids: [], permissions_by_brand: {} } };
    await flush(); slow.resolve(ok({ brand_id: brand, items: [reward({ version: 1 })], total_count: "1", limit: 20, offset: 0 })); await flush();
    expect(server.calls.filter((call) => call.init?.method === "GET")).toHaveLength(1);
    expect(byTestId(mounted.container, "rewards-revoke")).toBeNull();
    expect(textOf(mounted.container)).not.toContain(orderID);
    mounted.app.unmount();
  });

  it("changes language without making a reward write or silently refetching", async () => {
    const server = installFetch(); const mounted = mount(baseAccount); await flush();
    const count = server.calls.length;
    mounted.i18n.setLocale("en");
    await flush();
    expect(server.calls).toHaveLength(count);
    expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(0);
    mounted.app.unmount();
  });
});
