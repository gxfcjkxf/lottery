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
import type { ReportArchivePolicy, ReportArchiveTask } from "./report-archive-tasks-api";
import * as TasksState from "./report-archive-tasks-state";
import * as PolicyApi from "./report-archive-policy-api";
import * as PolicyState from "./report-archive-policy-state";

const actor = "33333333-3333-4333-8333-333333333333";
const brand = "11111111-1111-4111-8111-111111111111";
const taskId = "44444444-4444-4444-8444-444444444444";
const permissions = ["report_archive.view.brand", "report_archive_task.retry.brand"];
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: permissions } };
const calls: Array<{ method: string; args: unknown[] }> = [];
let nextRetry: (() => Promise<unknown>) | null = null;
let policyFailure: unknown = null, listFailure: unknown = null, readFailure: unknown = null;
let listResult: { brand_id: string; items: ReportArchiveTask[]; total_count: string; limit: number; offset: number };
let readResult: ReportArchiveTask;
let keyCounter = 0;

const policy: ReportArchivePolicy = { brand_id: brand, version: 3, daily_enabled: false, monthly_enabled: false, daily_start_period: null, monthly_start_period: null, timezone: "Asia/Singapore", audit_log_id: null, updated_at: "2026-10-03T00:00:00Z" };
function task(overrides: Partial<ReportArchiveTask> = {}): ReportArchiveTask {
  return { id: taskId, brand_id: brand, policy_version: 3, window: { kind: "daily", period_key: "2026-10-01", timezone: "Asia/Singapore", from: "2026-09-30T16:00:00Z", to: "2026-10-01T16:00:00Z" }, state: "failed", version: 4, attempt_count: 2, archive_id: null, last_error_code: "ARCHIVE_FAILED", creation_audit_log_id: "55555555-5555-4555-8555-555555555555", last_audit_log_id: "77777777-7777-4777-8777-777777777777", created_at: "2026-10-01T16:00:00Z", updated_at: "2026-10-03T00:00:00Z", ...overrides };
}
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason?: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; }); return { promise, resolve, reject }; }
function fresh() {
  calls.length = 0; nextRetry = null; policyFailure = listFailure = readFailure = null; keyCounter = 0;
  readResult = task(); listResult = { brand_id: brand, items: [task()], total_count: "21", limit: 20, offset: 0 };
}
function apiStub() {
  return {
    policy: async (...args: unknown[]) => { calls.push({ method: "policy", args }); if (policyFailure) throw policyFailure; return policy; },
    list: async (...args: unknown[]) => { calls.push({ method: "list", args }); if (listFailure) throw listFailure; return listResult; },
    read: async (...args: unknown[]) => { calls.push({ method: "read", args }); if (readFailure) throw readFailure; return readResult; },
    retry: async (...args: unknown[]) => { calls.push({ method: "retry", args }); return nextRetry ? nextRetry() : task({ state: "pending", version: 5, attempt_count: 2, last_error_code: null }); },
  };
}
type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _old, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => { if (node.parent) { const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); } node.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(node); else parent.children.splice(i, 0, node); },
  remove: (node) => { if (!node.parent) return; const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null, nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});
function compileComponent(): Component {
  const source = readFileSync(new URL("./ReportArchiveTasksManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "ReportArchiveTasksManagement.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "report-archive-tasks-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./report-archive-tasks-api": TasksApi, "./report-archive-tasks-state": TasksState, "./report-archive-policy-api": PolicyApi, "./report-archive-policy-state": PolicyState };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => { if (!(specifier in modules)) throw new Error(`Unmapped import ${specifier}`); return `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`; }).replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => { if (!(specifier in modules)) throw new Error(`Unmapped import ${specifier}`); return `const ${binding}=__modules[${JSON.stringify(specifier)}].default;`; }).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
vi.spyOn(TasksApi, "createReportArchiveTasksApi").mockImplementation(() => apiStub() as never);
vi.spyOn(AdminApi, "createIdempotencyKey").mockImplementation(() => `archive-retry-key-${String(++keyCounter).padStart(3, "0")}`);
const ComponentUnderTest = compileComponent();
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, predicate: (candidate: HostNode) => boolean): HostNode | null { if (predicate(node)) return node; for (const child of node.children) { const found = findNode(child, predicate); if (found) return found; } return null; }
function byTestId(root: HostNode, id: string): HostNode | null { return findNode(root, (node) => node.props["data-testid"] === id); }
function click(node: HostNode | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
function model(node: HostNode | null, value: unknown) { expect(node).not.toBeNull(); (node!.props["onUpdate:modelValue"] as ((next: unknown) => void) | undefined)?.(value); }
function mount(user = account, brandId = brand) {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  const scope = ref({ account: user, brandId }), container = element("root"), emitted: string[] = [], i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(ComponentUnderTest, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(container); return { app, scope, container, emitted, i18n };
}
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
async function selectFailedAndReview(mounted: ReturnType<typeof mount>, reason = "Verified source data") {
  const row = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes(taskId));
  click(row); await flush();
  model(byTestId(mounted.container, "archive-tasks-reason"), reason); await flush();
  click(byTestId(mounted.container, "archive-tasks-retry")); await flush();
  expect(byTestId(mounted.container, "archive-tasks-review")).not.toBeNull();
}
async function confirmAndSubmit(mounted: ReturnType<typeof mount>) {
  model(byTestId(mounted.container, "archive-tasks-confirm"), true); await flush();
  click(byTestId(mounted.container, "archive-tasks-submit")); await flush();
}
beforeEach(() => { fresh(); TasksState.clearAllReportArchiveTaskRetryIntents(); PolicyState.clearAllReportArchivePolicyIntents(); vi.mocked(TasksApi.createReportArchiveTasksApi).mockImplementation(() => apiStub() as never); });
afterEach(() => { TasksState.clearAllReportArchiveTaskRetryIntents(); PolicyState.clearAllReportArchivePolicyIntents(); calls.length = 0; vi.clearAllMocks(); vi.unstubAllGlobals(); });

describe("ReportArchiveTasksManagement SFC contract", () => {
  it("freezes unknown retries across unmount and locale changes, and replays only on explicit action", async () => {
    const first = mount(); await flush(); await selectFailedAndReview(first);
    const detail = textOf(byTestId(first.container, "archive-tasks-review")!);
    expect(detail).toContain(actor); expect(detail).toContain(brand); expect(detail).toContain(taskId); expect(detail).toContain("4"); expect(detail).toContain("Verified source data"); expect(detail).toContain("archive-retry-key-001");
    nextRetry = async () => { throw new TypeError("response lost"); };
    await confirmAndSubmit(first);
    const originalArgs = calls.find((call) => call.method === "retry")!.args;
    expect(originalArgs).toEqual([brand, taskId, { version: 4, reason: "Verified source data" }, "archive-retry-key-001", actor]);
    expect(byTestId(first.container, "archive-tasks-frozen")).not.toBeNull();
    first.app.unmount();
    const second = mount(); await flush();
    const count = calls.filter((call) => call.method === "retry").length;
    second.i18n.setLocale("en"); await flush();
    expect(textOf(second.container)).toContain("Automatic archive tasks");
    expect(textOf(byTestId(second.container, "archive-tasks-frozen")!)).toContain("The account, brand, task, version, reason and original idempotency key stay in current-session memory.");
    expect(calls.filter((call) => call.method === "retry")).toHaveLength(count);
    expect(byTestId(second.container, "archive-tasks-replay")!.props.disabled).toBe(true);
    expect(byTestId(second.container, "archive-tasks-replay-check")).not.toBeNull();
    model(byTestId(second.container, "archive-tasks-replay-check"), true); await flush();
    expect(byTestId(second.container, "archive-tasks-replay")!.props.disabled).toBe(false);
    click(byTestId(second.container, "archive-tasks-replay")); await flush();
    expect(calls.filter((call) => call.method === "retry")).toHaveLength(count + 1);
    expect(calls.filter((call) => call.method === "retry").at(-1)?.args).toEqual(originalArgs);
    second.app.unmount();
  });

  it("requires list and original task reload plus human discard after 409", async () => {
    const mounted = mount(); await flush(); await selectFailedAndReview(mounted);
    nextRetry = async () => { throw new AdminApiError("task version conflict", 409); };
    await confirmAndSubmit(mounted);
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)?.phase).toBe("conflict");
    const before = calls.length;
    click(byTestId(mounted.container, "archive-tasks-reload-frozen")); await flush();
    expect(calls.slice(before).filter((call) => ["list", "read"].includes(call.method)).map((call) => call.method).sort()).toEqual(["list", "read"]);
    expect((byTestId(mounted.container, "archive-tasks-discard")!.props.disabled)).toBe(true);
    model(byTestId(mounted.container, "archive-tasks-discard-check"), true); await flush();
    click(byTestId(mounted.container, "archive-tasks-discard")); await flush();
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)).toBeNull();
    mounted.app.unmount();
  });

  it("keeps original ACK separate from failed latest list and detail reads", async () => {
    const mounted = mount(); await flush(); await selectFailedAndReview(mounted);
    listFailure = new AdminApiError("latest list unavailable", 503); readFailure = new AdminApiError("latest task unavailable", 503);
    await confirmAndSubmit(mounted);
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)?.phase).toBe("acknowledged");
    expect(byTestId(mounted.container, "archive-tasks-list-error")).not.toBeNull();
    expect(byTestId(mounted.container, "archive-tasks-detail-error")).not.toBeNull();
    expect(byTestId(mounted.container, "archive-tasks-frozen")).not.toBeNull();
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)?.phase).toBe("acknowledged");
    expect(textOf(byTestId(mounted.container, "archive-tasks-ack-receipt")!)).toContain("pending");
    expect(textOf(byTestId(mounted.container, "archive-tasks-frozen")!)).not.toContain("Retry outcome unknown");
    expect(calls.filter((call) => call.method === "retry")).toHaveLength(1);
    mounted.app.unmount();
  });

  it("restores the exact retry after a lost ACK and keeps its pending ACK separate from a completed worker query", async () => {
    const first = mount(); await flush(); await selectFailedAndReview(first, "Reconcile archive source");
    nextRetry = async () => { throw new TypeError("retry response was lost"); };
    await confirmAndSubmit(first);
    const originalArgs = calls.find((call) => call.method === "retry")!.args;
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)).toMatchObject({ phase: "unknown", version: 4, reason: "Reconcile archive source", key: "archive-retry-key-001" });
    first.app.unmount();

    const completed = task({ state: "completed", version: 5, attempt_count: 3, archive_id: "88888888-8888-4888-8888-888888888888", last_error_code: null });
    listResult = { brand_id: brand, items: [completed], total_count: "21", limit: 20, offset: 0 };
    readResult = completed;
    const returnVisit = mount(); returnVisit.i18n.setLocale("en"); await flush();
    expect(textOf(returnVisit.container)).toContain("completed · v5");
    expect(byTestId(returnVisit.container, "archive-tasks-list")?.props["aria-label"]).toContain("Automatic archive task list");
    expect(findNode(returnVisit.container, (node) => node.tag === "button" && textOf(node).includes(taskId))?.props["aria-label"]).toContain(taskId);
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)?.phase).toBe("unknown");
    expect(calls.filter((call) => call.method === "retry")).toHaveLength(1);

    const originalPendingAck = task({ state: "pending", version: 5, attempt_count: 2, last_error_code: null });
    nextRetry = async () => originalPendingAck;
    click(byTestId(returnVisit.container, "archive-tasks-replay")); await flush();
    expect(byTestId(returnVisit.container, "archive-tasks-replay")!.props.disabled).toBe(true);
    model(byTestId(returnVisit.container, "archive-tasks-replay-check"), true); await flush();
    click(byTestId(returnVisit.container, "archive-tasks-replay")); await flush();
    expect(calls.filter((call) => call.method === "retry")).toHaveLength(2);
    expect(calls.filter((call) => call.method === "retry").at(-1)?.args).toEqual(originalArgs);
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)).toMatchObject({ phase: "acknowledged", version: 4, reason: "Reconcile archive source", key: "archive-retry-key-001" });
    expect(textOf(byTestId(returnVisit.container, "archive-tasks-frozen")!)).toContain("Original retry acknowledged");
    const ack = byTestId(returnVisit.container, "archive-tasks-receipt")!;
    expect(textOf(ack)).toContain("pending"); expect(textOf(ack)).toContain("5");
    expect(textOf(byTestId(returnVisit.container, "archive-tasks-detail")!)).toContain("completed");
    expect(textOf(ack)).toContain("current task query shows a later worker state");
    expect(textOf(returnVisit.container)).toContain("The original retry request was acknowledged.");
    expect(calls.filter((call) => call.method === "read").at(-1)?.args).toEqual([brand, taskId]);

    model(byTestId(returnVisit.container, "archive-tasks-discard-check"), true); await flush();
    click(byTestId(returnVisit.container, "archive-tasks-discard")); await flush();
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)).toBeNull();
    returnVisit.app.unmount();
  });

  it("keeps policy, list and detail failures independent", async () => {
    policyFailure = new AdminApiError("policy failed", 503); listFailure = new AdminApiError("list failed", 503);
    const mounted = mount(); await flush();
    expect(byTestId(mounted.container, "archive-tasks-policy-error")).not.toBeNull();
    expect(byTestId(mounted.container, "archive-tasks-list-error")).not.toBeNull();
    policyFailure = listFailure = null; click(byTestId(mounted.container, "archive-tasks-refresh")); await flush(); await selectFailedAndReview(mounted);
    readFailure = new AdminApiError("detail failed", 503);
    const row = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes(taskId)); click(row); await flush();
    expect(byTestId(mounted.container, "archive-tasks-detail-error")).not.toBeNull();
    mounted.app.unmount();
  });

  it("clears every intent on a current 401 but ignores a stale write 401 after session generation changes", async () => {
    TasksState.setReportArchiveTaskRetryIntent(TasksState.createReportArchiveTaskRetryIntent(actor, brand, taskId, 4, "existing retry", "existing-retry-key-01")!);
    const current = mount(); await flush(); listFailure = new AdminApiError("expired", 401);
    click(byTestId(current.container, "archive-tasks-refresh")); await flush();
    expect(TasksState.getReportArchiveTaskRetryIntents(actor, brand)).toHaveLength(0); expect(current.emitted).toEqual(["session-invalid"]); current.app.unmount();

    listFailure = null;
    const deferredRetry = deferred<unknown>(); nextRetry = () => deferredRetry.promise;
    const old = mount(); await flush(); await selectFailedAndReview(old); await confirmAndSubmit(old);
    TasksState.clearAllReportArchiveTaskRetryIntents();
    const newIntent = TasksState.createReportArchiveTaskRetryIntent(actor, brand, taskId, 9, "new session request", "new-session-retry-01")!;
    TasksState.setReportArchiveTaskRetryIntent(newIntent);
    deferredRetry.reject(new AdminApiError("old session expired", 401)); await flush();
    expect(TasksState.getReportArchiveTaskRetryIntent(actor, brand, taskId)?.key).toBe("new-session-retry-01"); expect(old.emitted).toEqual([]);
    old.app.unmount();
  });

  it("denies super-admin brand access and keeps mapped view-only members read-only", async () => {
    const superAdmin: AdminAccount = { id: actor, super_admin: true, brand_ids: [], permissions: [], platform_permissions: ["report_archive.view.platform", "report_archive_task.retry.brand"] };
    const superView = mount(superAdmin); superView.i18n.setLocale("en"); await flush();
    expect(byTestId(superView.container, "archive-tasks-retry")).toBeNull();
    expect(calls.some((call) => call.method === "retry")).toBe(false); superView.app.unmount();
    const viewOnly: AdminAccount = { ...account, permissions_by_brand: { [brand]: ["report_archive.view.brand"] } };
    const memberView = mount(viewOnly); memberView.i18n.setLocale("en"); await flush();
    expect(textOf(memberView.container)).toContain("manual retry permission is not granted");
    click(findNode(memberView.container, (node) => node.tag === "button" && textOf(node).includes(taskId))); await flush();
    expect((byTestId(memberView.container, "archive-tasks-retry")!.props.disabled)).toBe(true);
    memberView.app.unmount();
  });

  it("does not infer global state from the current page and isolates late scope responses", async () => {
    const delayedList = deferred<{ brand_id: string; items: ReportArchiveTask[]; total_count: string; limit: number; offset: number }>();
    let delay = false;
    const original = apiStub;
    vi.mocked(TasksApi.createReportArchiveTasksApi).mockImplementation(() => {
      const stub = original(); const list = stub.list;
      stub.list = async (...args: unknown[]) => { if (delay) { calls.push({ method: "list-delayed", args }); return delayedList.promise; } return list(...args); };
      return stub as never;
    });
    const mounted = mount(); mounted.i18n.setLocale("en"); await flush();
    expect(textOf(mounted.container)).toContain("Statuses below describe this page only");
    expect(textOf(mounted.container)).not.toContain("All tasks completed");
    delay = true; click(byTestId(mounted.container, "archive-tasks-refresh")); await flush();
    listResult = { brand_id: "22222222-2222-4222-8222-222222222222", items: [], total_count: "0", limit: 20, offset: 0 };
    mounted.scope.value = { account, brandId: "22222222-2222-4222-8222-222222222222" }; await flush();
    delayedList.resolve({ brand_id: brand, items: [task()], total_count: "21", limit: 20, offset: 0 }); await flush();
    expect(textOf(mounted.container)).not.toContain(taskId);
    mounted.app.unmount();
  });
});
