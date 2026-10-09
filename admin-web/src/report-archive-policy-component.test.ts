import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import ts from "typescript";
import { compileScript, parse } from "vue/compiler-sfc";
import * as AdminApi from "./admin-api";
import { AdminApiError, type AdminAccount } from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import * as TasksApi from "./report-archive-tasks-api";
import * as TasksState from "./report-archive-tasks-state";
import type { ReportArchivePolicy } from "./report-archive-tasks-api";
import * as PolicyApi from "./report-archive-policy-api";
import type { UpdateReportArchivePolicyInput } from "./report-archive-policy-api";
import * as PolicyState from "./report-archive-policy-state";

const actor = "33333333-3333-4333-8333-333333333333";
const brand = "11111111-1111-4111-8111-111111111111";
const otherBrand = "22222222-2222-4222-8222-222222222222";
const audit = "55555555-5555-4555-8555-555555555555";
const permissions = ["report_archive.view.brand", "report_archive_task.retry.brand", "report_archive_policy.write.brand"];
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand, otherBrand], permissions: [], permissions_by_brand: { [brand]: permissions, [otherBrand]: permissions } };
const calls: Array<{ method: string; args: unknown[] }> = [];
const mountedApps: Array<{ unmount: () => void }> = [];
let policyResult: ReportArchivePolicy;
let policyFailure: unknown = null;
let listFailure: unknown = null;
let readFailure: unknown = null;
let updateFailure: unknown = null;
let nextUpdate: (() => Promise<ReportArchivePolicy>) | null = null;
let keyCounter = 0;

function policy(version = 1, overrides: Partial<ReportArchivePolicy> = {}): ReportArchivePolicy {
  const enabled = version > 1;
  return {
    brand_id: brand, version, daily_enabled: enabled, monthly_enabled: false,
    daily_start_period: enabled ? "2026-10-09" : null, monthly_start_period: null,
    timezone: "Asia/Singapore", audit_log_id: enabled ? audit : null,
    updated_at: "2026-10-09T00:00:00Z", ...overrides,
  };
}

function fresh() {
  calls.length = 0;
  policyResult = policy();
  policyFailure = listFailure = readFailure = updateFailure = null;
  nextUpdate = null;
  keyCounter = 0;
}

function tasksApiStub() {
  return {
    policy: async (...args: unknown[]) => { calls.push({ method: "policy", args }); if (policyFailure) throw policyFailure; return policyResult; },
    list: async (...args: unknown[]) => { calls.push({ method: "list", args }); if (listFailure) throw listFailure; return { brand_id: brand, items: [], total_count: "0", limit: 20, offset: 0 }; },
    read: async (...args: unknown[]) => { calls.push({ method: "read", args }); if (readFailure) throw readFailure; throw new Error("unexpected task read"); },
  };
}

function policyApiStub() {
  return {
    update: async (...args: unknown[]) => {
      calls.push({ method: "update", args });
      if (updateFailure) throw updateFailure;
      if (nextUpdate) return nextUpdate();
      const [, input] = args as [string, UpdateReportArchivePolicyInput, string, string];
      return policy(input.version + 1, {
        daily_enabled: input.daily_enabled,
        monthly_enabled: input.monthly_enabled,
        daily_start_period: input.daily_enabled ? "2026-10-09" : null,
        monthly_start_period: input.monthly_enabled ? "2026-10" : null,
        audit_log_id: audit,
      });
    },
  };
}

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; listeners: Record<string, Array<(event: any) => void>>; addEventListener: (name: string, listener: (event: any) => void) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode {
  const node = { tag, props: {}, children: [], text: "", value: "", checked: false, listeners: {}, addEventListener(name: string, listener: (event: any) => void) { (this.listeners[name] ??= []).push(listener); }, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter(child => child.tag === "option"); } } as HostNode;
  return node;
}
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element, createText: text => ({ ...element("#text"), text }), createComment: text => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _old, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); if (key === "checked") node.checked = Boolean(value); },
  insert: (node, parent, anchor) => { if (node.parent) { const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); } node.parent = parent; const index = anchor ? parent.children.indexOf(anchor) : -1; if (index < 0) parent.children.push(node); else parent.children.splice(index, 0, node); },
  remove: node => { if (!node.parent) return; const index = node.parent.children.indexOf(node); if (index >= 0) node.parent.children.splice(index, 1); node.parent = undefined; },
  parentNode: node => node.parent ?? null, nextSibling: node => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});

function compileComponent(): Component {
  const source = readFileSync(new URL("./ReportArchiveTasksManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "ReportArchiveTasksManagement.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "report-archive-policy-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = {
    vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n,
    "./report-archive-tasks-api": TasksApi, "./report-archive-tasks-state": TasksState,
    "./report-archive-policy-api": PolicyApi, "./report-archive-policy-state": PolicyState,
  };
  const body = javascript
    .replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) => {
      if (!(specifier in modules)) throw new Error(`Unmapped test component import: ${specifier}`);
      return `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`;
    })
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, binding: string, specifier: string) => {
      if (!(specifier in modules)) throw new Error(`Unmapped test component import: ${specifier}`);
      return `const ${binding}=__modules[${JSON.stringify(specifier)}].default;`;
    })
    .replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

vi.spyOn(TasksApi, "createReportArchiveTasksApi").mockImplementation(() => tasksApiStub() as never);
vi.spyOn(PolicyApi, "createReportArchivePolicyApi").mockImplementation(() => policyApiStub() as never);
vi.spyOn(AdminApi, "createIdempotencyKey").mockImplementation(() => `archive-policy-key-${String(++keyCounter).padStart(2, "0")}`);
const ComponentUnderTest = compileComponent();

function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, predicate: (candidate: HostNode) => boolean): HostNode | null { if (predicate(node)) return node; for (const child of node.children) { const found = findNode(child, predicate); if (found) return found; } return null; }
function byTestId(root: HostNode, id: string): HostNode | null { return findNode(root, node => node.props["data-testid"] === id); }
function click(node: HostNode | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
function model(node: HostNode | null, value: unknown) {
  expect(node).not.toBeNull();
  const update = node!.props["onUpdate:modelValue"] as ((next: unknown) => void) | undefined;
  if (update) update(value);
  if (node!.tag === "input" && node!.props.type === "checkbox") {
    node!.checked = Boolean(value);
    for (const listener of node!.listeners.change ?? []) listener({ target: node!, stopPropagation() {} });
  } else if (node!.tag === "textarea" || node!.tag === "input") {
    node!.value = String(value ?? "");
    for (const listener of node!.listeners.input ?? []) listener({ target: node!, stopPropagation() {} });
  }
}
function mount(user = account, brandId = brand) {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  const scope = ref({ account: user, brandId }), container = element("root"), emitted: string[] = [], i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(container); mountedApps.push(app); return { app, scope, container, emitted, i18n };
}
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function currentIntent() { return PolicyState.getReportArchivePolicyIntent(actor, brand); }
function updateCalls() { return calls.filter(call => call.method === "update"); }
async function startReview(mounted: ReturnType<typeof mount>, daily = true, monthly = false, reason = "Reviewed archive schedule") {
  click(byTestId(mounted.container, "archive-policy-editor")); await flush();
  model(byTestId(mounted.container, "archive-policy-daily"), daily);
  model(byTestId(mounted.container, "archive-policy-monthly"), monthly);
  model(byTestId(mounted.container, "archive-policy-reason"), reason); await flush();
  expect(byTestId(mounted.container, "archive-policy-save")!.props.disabled).toBe(false);
  click(byTestId(mounted.container, "archive-policy-save")); await flush();
  const review = byTestId(mounted.container, "archive-policy-review");
  expect(review).not.toBeNull();
  expect(textOf(review!)).toContain("确认策略更新");
}
async function submitReviewed(mounted: ReturnType<typeof mount>) {
  model(byTestId(mounted.container, "archive-policy-confirm"), true); await flush();
  click(byTestId(mounted.container, "archive-policy-submit")); await flush();
}

beforeEach(() => {
  fresh(); PolicyState.clearAllReportArchivePolicyIntents(); TasksState.clearAllReportArchiveTaskRetryIntents();
  vi.mocked(TasksApi.createReportArchiveTasksApi).mockImplementation(() => tasksApiStub() as never);
  vi.mocked(PolicyApi.createReportArchivePolicyApi).mockImplementation(() => policyApiStub() as never);
});
afterEach(() => { for (const app of mountedApps.splice(0)) app.unmount(); PolicyState.clearAllReportArchivePolicyIntents(); TasksState.clearAllReportArchiveTaskRetryIntents(); calls.length = 0; vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("ReportArchiveTasksManagement archive policy editor contract", () => {
  it("keeps the existing policy view, starts disabled, and performs no automatic PUT", async () => {
    const mounted = mount(); await flush();
    expect(byTestId(mounted.container, "archive-tasks-policy")).not.toBeNull();
    const existingPolicy = textOf(byTestId(mounted.container, "archive-tasks-policy")!);
    expect(existingPolicy).toContain("—");
    expect(byTestId(mounted.container, "archive-policy-editor")).not.toBeNull();
    expect(byTestId(mounted.container, "archive-policy-daily")!.checked).toBe(false);
    expect(byTestId(mounted.container, "archive-policy-monthly")!.checked).toBe(false);
    expect(calls.filter(call => call.method === "policy")).toHaveLength(1);
    expect(updateCalls()).toHaveLength(0);
    mounted.app.unmount();
  });

  it("requires reason, review and explicit confirmation, then sends the exact actor-brand snapshot", async () => {
    const mounted = mount(); await flush(); await startReview(mounted, true, true, "Enable daily and monthly archive");
    const review = byTestId(mounted.container, "archive-policy-review")!;
    expect(textOf(review)).toContain(actor); expect(textOf(review)).toContain(brand); expect(textOf(review)).toContain("1");
    expect(textOf(review)).toContain("Enable daily and monthly archive");
    expect(textOf(review)).toContain("2026-10-09"); expect(textOf(review)).toContain("2026-10");
    expect(updateCalls()).toHaveLength(0);
    expect((byTestId(mounted.container, "archive-policy-submit")!.props.disabled)).toBe(true);
    await submitReviewed(mounted);
    expect(updateCalls()).toHaveLength(1);
    expect(updateCalls()[0]!.args).toEqual([brand, { version: 1, daily_enabled: true, monthly_enabled: true, reason: "Enable daily and monthly archive" }, "archive-policy-key-01", actor]);
    expect(byTestId(mounted.container, "archive-policy-receipt")).not.toBeNull();
    mounted.app.unmount();
  });

  it("freezes an unknown write across a newer off-policy GET and replays only the original request on explicit confirmation", async () => {
    const first = mount(); await flush(); await startReview(first, true, false, "Enable daily archive");
    nextUpdate = async () => { throw new TypeError("response lost"); };
    await submitReviewed(first);
    const original = updateCalls()[0]!.args;
    expect(currentIntent()).toMatchObject({ phase: "unknown", version: 1, daily_enabled: true, monthly_enabled: false, reason: "Enable daily archive", key: "archive-policy-key-01" });
    expect(byTestId(first.container, "archive-policy-frozen")).not.toBeNull();
    first.app.unmount();

    policyResult = policy(3, { daily_enabled: false, monthly_enabled: false, daily_start_period: "2026-10-09", monthly_start_period: null });
    const returned = mount(); await flush();
    expect(textOf(byTestId(returned.container, "archive-policy-frozen")!)).toContain("archive-policy-key-01");
    expect(currentIntent()?.version).toBe(1);
    expect(byTestId(returned.container, "archive-policy-replay")!.props.disabled).toBe(true);
    model(byTestId(returned.container, "archive-policy-replay-check"), true); await flush();
    nextUpdate = null;
    click(byTestId(returned.container, "archive-policy-replay")); await flush();
    expect(updateCalls()).toHaveLength(2);
    expect(updateCalls()[1]!.args).toEqual(original);
    expect(currentIntent()).toMatchObject({ phase: "acknowledged", version: 1, receipt: { version: 2, daily_enabled: true, monthly_enabled: false, audit_log_id: audit } });
    returned.app.unmount();
  });

  it("preserves a valid ACK receipt when the follow-up GET fails", async () => {
    const mounted = mount(); await flush(); await startReview(mounted, true, false, "Enable archive");
    policyFailure = new AdminApiError("latest policy unavailable", 503);
    await submitReviewed(mounted);
    expect(currentIntent()).toMatchObject({ phase: "acknowledged", version: 1, key: "archive-policy-key-01", receipt: { version: 2, daily_enabled: true, audit_log_id: audit } });
    expect(byTestId(mounted.container, "archive-policy-receipt")).not.toBeNull();
    expect(byTestId(mounted.container, "archive-tasks-policy-error")).not.toBeNull();
    expect(updateCalls()).toHaveLength(1);
    mounted.app.unmount();
  });

  it("requires fresh policy reload and explicit discard after 409", async () => {
    const mounted = mount(); await flush(); await startReview(mounted, true, false, "Enable archive");
    updateFailure = new AdminApiError("policy conflict", 409);
    await submitReviewed(mounted);
    expect(currentIntent()?.phase).toBe("conflict");
    const before = calls.length;
    policyResult = policy(4, { daily_enabled: false, monthly_enabled: false, daily_start_period: null, monthly_start_period: null });
    click(byTestId(mounted.container, "archive-policy-reload-frozen")); await flush();
    expect(calls.slice(before).filter(call => call.method === "policy").length).toBeGreaterThan(0);
    expect(byTestId(mounted.container, "archive-policy-discard")!.props.disabled).toBe(true);
    model(byTestId(mounted.container, "archive-policy-discard-check"), true); await flush();
    click(byTestId(mounted.container, "archive-policy-discard")); await flush();
    expect(currentIntent()).toBeNull();
    mounted.app.unmount();
  });

  it("blocks writes after load failures and denies platform and super-admin brand access", async () => {
    policyFailure = new AdminApiError("policy unavailable", 503);
    const failed = mount(); await flush();
    expect(byTestId(failed.container, "archive-policy-save")?.props.disabled).toBe(true);
    expect(updateCalls()).toHaveLength(0); failed.app.unmount();
    policyFailure = null;

    const platform: AdminAccount = { ...account, super_admin: true, brand_ids: [], platform_permissions: ["report_archive.view.platform", "report_archive_policy.write.brand"] };
    const superAdmin = mount(platform); await flush();
    expect(byTestId(superAdmin.container, "archive-policy-editor")).toBeNull();
    expect(updateCalls()).toHaveLength(0); superAdmin.app.unmount();
    const viewOnly: AdminAccount = { ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand"] } };
    const readOnly = mount(viewOnly); await flush();
    expect(byTestId(readOnly.container, "archive-policy-daily")?.props.disabled).toBe(true);
    expect(updateCalls()).toHaveLength(0); readOnly.app.unmount();
  });

  it.each(["brand", "account", "permission"] as const)("ignores a late policy GET after a %s scope change", async change => {
    const pending = deferred<ReportArchivePolicy>();
    let reads = 0;
    vi.mocked(TasksApi.createReportArchiveTasksApi).mockImplementation(() => ({
      policy: async (...args: unknown[]) => {
        calls.push({ method: "policy", args });
        if (reads++ === 0) return pending.promise;
        return policy(7, { brand_id: args[0] as string, daily_enabled: false, daily_start_period: null, audit_log_id: audit });
      },
      list: async (...args: unknown[]) => { calls.push({ method: "list", args }); return { brand_id: args[0] as string, items: [], total_count: "0", limit: 20, offset: 0 }; },
      read: async (...args: unknown[]) => { calls.push({ method: "read", args }); throw new Error("unused"); },
    }) as never);
    const mounted = mount(); await flush();
    if (change === "brand") mounted.scope.value = { account, brandId: otherBrand };
    if (change === "account") mounted.scope.value = { account: { ...account, id: "44444444-4444-4444-8444-444444444444" }, brandId: brand };
    if (change === "permission") mounted.scope.value = { account: { ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand"] } }, brandId: brand };
    await flush(); pending.resolve(policy(99)); await flush();
    const rendered = textOf(mounted.container);
    expect(rendered).not.toContain("99");
    if (change !== "permission") expect(rendered).toContain("7");
    mounted.app.unmount();
  });

  it("ignores stale policy reads after scope changes, clears on current 401, and protects a newer session from late 401 and 409", async () => {
    PolicyState.setReportArchivePolicyIntent(PolicyState.createReportArchivePolicyIntent(actor, brand, 1, true, false, "Existing intent", "existing-policy-key")!);
    TasksState.setReportArchiveTaskRetryIntent(TasksState.createReportArchiveTaskRetryIntent(actor, brand, "44444444-4444-4444-8444-444444444444", 1, "Existing task intent", "existing-task-key")!);
    const current = mount(); await flush(); policyFailure = new AdminApiError("expired", 401);
    click(byTestId(current.container, "archive-tasks-refresh")); await flush();
    expect(PolicyState.getReportArchivePolicyIntents(actor)).toHaveLength(0);
    expect(TasksState.getReportArchiveTaskRetryIntents(actor, brand)).toHaveLength(0);
    expect(current.emitted).toContain("session-invalid"); current.app.unmount();

    policyFailure = null;
    const pendingWrite = deferred<ReportArchivePolicy>();
    nextUpdate = () => pendingWrite.promise;
    const old = mount(); await flush(); await startReview(old); await submitReviewed(old);
    const lateWrite = calls.filter(call => call.method === "update").at(-1)!;
    PolicyState.clearAllReportArchivePolicyIntents();
    const newIntent = PolicyState.createReportArchivePolicyIntent(actor, brand, 9, false, true, "New session", "new-session-key")!;
    PolicyState.setReportArchivePolicyIntent(newIntent);
    pendingWrite.reject(new AdminApiError("old session expired", 401)); await flush();
    expect(PolicyState.getReportArchivePolicyIntent(actor, brand)?.key).toBe("new-session-key");
    expect(old.emitted).toEqual([]);
    expect(updateCalls().at(-1)?.args).toEqual(lateWrite.args);
    old.app.unmount();
  });

  it("does not attach a late successful ACK to policy or task intents from a newer session", async () => {
    const pendingAck = deferred<ReportArchivePolicy>();
    nextUpdate = () => pendingAck.promise;
    const mounted = mount(); await flush(); await startReview(mounted, true, false, "Enable archive");
    await submitReviewed(mounted);
    expect(currentIntent()).toMatchObject({ phase: "unknown", version: 1, key: "archive-policy-key-01" });

    PolicyState.clearAllReportArchivePolicyIntents();
    TasksState.clearAllReportArchiveTaskRetryIntents();
    const newerPolicyIntent = PolicyState.createReportArchivePolicyIntent(actor, brand, 9, false, true, "New policy request", "new-policy-session-key")!;
    const newerTaskIntent = TasksState.createReportArchiveTaskRetryIntent(actor, brand, "44444444-4444-4444-8444-444444444444", 9, "New task request", "new-task-session-key")!;
    PolicyState.setReportArchivePolicyIntent(newerPolicyIntent);
    TasksState.setReportArchiveTaskRetryIntent(newerTaskIntent);

    pendingAck.resolve(policy(2, { daily_enabled: true, monthly_enabled: false, daily_start_period: "2026-10-09", monthly_start_period: null, audit_log_id: audit }));
    await flush();
    expect(currentIntent()).toMatchObject({ phase: "unknown", version: 9, daily_enabled: false, monthly_enabled: true, key: "new-policy-session-key" });
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, "44444444-4444-4444-8444-444444444444")?.key).toBe("new-task-session-key");
    expect(textOf(mounted.container)).not.toContain("Original update ACK (complete policy)");
    mounted.app.unmount();
  });

  it("keeps English labels after locale changes and does not automatically write or rebind a late conflict", async () => {
    const mounted = mount(); await flush(); await startReview(mounted, true, false, "Enable daily archive");
    const pendingWrite = deferred<ReportArchivePolicy>(); nextUpdate = () => pendingWrite.promise;
    await submitReviewed(mounted);
    expect(currentIntent()?.phase).toBe("unknown");
    const count = updateCalls().length;
    mounted.i18n.setLocale("en"); await flush();
    expect(textOf(mounted.container)).toContain("Automatic archive policy");
    expect(textOf(byTestId(mounted.container, "archive-policy-frozen")!)).toContain("archive-policy-key-01");
    expect(updateCalls()).toHaveLength(count);
    PolicyState.clearAllReportArchivePolicyIntents();
    const replacement = PolicyState.createReportArchivePolicyIntent(actor, brand, 8, false, true, "Replacement request", "replacement-key")!;
    PolicyState.setReportArchivePolicyIntent(replacement);
    pendingWrite.reject(new AdminApiError("late conflict", 409)); await flush();
    expect(currentIntent()?.key).toBe("replacement-key"); expect(currentIntent()?.phase).toBe("unknown");
    expect(updateCalls()).toHaveLength(count);
    mounted.app.unmount();
  });
});

function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason?: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; }); return { promise, resolve, reject }; }
