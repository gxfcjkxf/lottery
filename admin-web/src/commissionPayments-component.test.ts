import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as PaymentApi from "./commissionPayments-api";
import * as PaymentState from "./commissionPayments-state";
import type { CommissionPayment, CommissionPaymentPolicy } from "./commissionPayments-api";

const brand = "11111111-1111-4111-8111-111111111111";
const actor = "33333333-3333-4333-8333-333333333333";
const paymentId = "44444444-4444-4444-8444-444444444444";
const cycleId = "55555555-5555-4555-8555-555555555555";
const runId = "66666666-6666-4666-8666-666666666666";
const auditId = "77777777-7777-4777-8777-777777777777";
const timestamp = "2026-10-07T00:00:00Z";
const baseAccount: AdminAccount = {
  id: actor, super_admin: false, brand_ids: [brand], permissions: [],
  permissions_by_brand: { [brand]: ["commission.view.brand", "commission_payment_policy.write.brand", "commission_payment.approve.brand", "commission_payment.retry.brand"] },
};
const PaymentComponent = compileComponent();

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
const renderer = createRenderer<HostNode, HostNode>({
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (node, text) => { node.text = text; }, setElementText: (node, text) => { node.text = text; node.children = []; },
  patchProp: (node, key, _oldValue, value) => { node.props[key] = value; if (key === "value") node.value = String(value ?? ""); },
  insert: (node, parent, anchor) => { if (node.parent) { const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); } node.parent = parent; const i = anchor ? parent.children.indexOf(anchor) : -1; if (i < 0) parent.children.push(node); else parent.children.splice(i, 0, node); },
  remove: (node) => { if (!node.parent) return; const i = node.parent.children.indexOf(node); if (i >= 0) node.parent.children.splice(i, 1); node.parent = undefined; },
  parentNode: (node) => node.parent ?? null, nextSibling: (node) => node.parent ? node.parent.children[node.parent.children.indexOf(node) + 1] ?? null : null,
});

function compileComponent(): Component {
  const source = readFileSync(new URL("./CommissionPaymentsManagement.vue", import.meta.url), "utf8");
  const descriptor = parseVue(source, {filename: "CommissionPaymentsManagement.vue"}).descriptor;
  const compiled = compileVueScript(descriptor, {id: "commission-payments-component-test", inlineTemplate: true}).content;
  const javascript = transpileScript(compiled);
  // Child behavior has its own real component tests and browser workflow;
  // isolate these payout control tests from the child's additional reads.
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commissionPayments-api": PaymentApi, "./commissionPayments-state": PaymentState,
    "./CommissionAdjustmentsManagement.vue": { default: { render: () => null } } };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, bindings: string, specifier: string) => {
    if (!(specifier in modules)) throw new Error(`Unmapped test import: ${specifier}`);
    return `const {${bindings.replace(/\s+as\s+/g, ": ")}} = __modules[${JSON.stringify(specifier)}];`;
  }).replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_match, binding: string, specifier: string) => {
    if (!(specifier in modules)) throw new Error(`Unmapped test import: ${specifier}`);
    return `const ${binding} = __modules[${JSON.stringify(specifier)}].default;`;
  }).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

// Import compiler helpers here to keep the SFC test in the same no-DOM renderer used by sibling tests.
import { compileScript as compileVueScript, parse as parseVue } from "vue/compiler-sfc";
import ts from "typescript";
function transpileScript(source: string): string { return ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText; }

function policy(enabled = false, version = 1): CommissionPaymentPolicy { return { brand_id: brand, version, enabled, audit_log_id: version === 1 ? "" : auditId, updated_at: timestamp }; }
function payment(overrides: Partial<CommissionPayment> = {}): CommissionPayment {
  return { id: paymentId, brand_id: brand, cycle_id: cycleId, run_id: runId, state: "failed", payout_mode: "automatic", version: 2, evidence_epoch: "3", total_points: "10", paid_points: "0", target_count: "2", paid_count: "0", creation_audit_log_id: auditId, last_error_code: "POST_FAILED", created_at: timestamp, updated_at: timestamp, ...overrides };
}
function ok(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data }), { status }); }
function failure(status: number) { return new Response(JSON.stringify({ success: false, error: { code: "CONFLICT", message: "Current version changed" } }), { status }); }
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, match: (candidate: HostNode) => boolean): HostNode | null { if (match(node)) return node; for (const child of node.children) { const found = findNode(child, match); if (found) return found; } return null; }
function byTestId(root: HostNode, id: string): HostNode | null { return findNode(root, (node) => node.props["data-testid"] === id); }
function button(root: HostNode, text: string): HostNode | null { return findNode(root, (node) => node.tag === "button" && textOf(node).includes(text)); }
function click(node: HostNode | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
function setModel(node: HostNode | null, value: unknown) { expect(node).not.toBeNull(); (node!.props["onUpdate:modelValue"] as ((next: unknown) => void) | undefined)?.(value); }
async function flush() { for (let i = 0; i < 14; i++) await Promise.resolve(); await nextTick(); }
function mount(account: AdminAccount, brandId = brand, brandStatus = "active") {
  const scope = ref({ account, brandId, brandStatus }), container = element("root"), emitted: string[] = [];
  const app = renderer.createApp({ setup: () => () => h(PaymentComponent, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, createAdminI18n()); app.mount(container);
  return { app, container, scope, emitted };
}
function installFetch(options: { current?: CommissionPayment; onRetry?: (call: { url: string; init?: RequestInit }) => Promise<Response> | Response; onPolicyPut?: (body: unknown) => Response; delayFirstDetail?: ReturnType<typeof deferred<Response>> } = {}) {
  vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
  let current = options.current ?? payment(), policyValue = policy(), detailReads = 0;
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input), method = init?.method ?? "GET"; calls.push({ url, init });
    if (url.endsWith("commission-payment-policy") && method === "GET") return ok(policyValue);
    if (url.endsWith("commission-payment-policy") && method === "PUT") { const body = JSON.parse(String(init?.body)); const response = options.onPolicyPut?.(body) ?? ok({ ...policyValue, version: body.version + 1, enabled: body.enabled, audit_log_id: auditId }); if (response.ok) policyValue = { ...policyValue, version: body.version + 1, enabled: body.enabled, audit_log_id: auditId }; return response; }
    if (url.includes("/commission-payments?") && method === "GET") return ok({ brand_id: brand, items: [current], total_count: "1", limit: 20, offset: Number(new URL(url, "http://local").searchParams.get("offset")) });
    if (url.endsWith(`/commission-payments/${paymentId}`) && method === "GET") { detailReads++; if (detailReads === 1 && options.delayFirstDetail) return options.delayFirstDetail.promise; return ok(current); }
    if (url.endsWith(`/commission-payments/${paymentId}/retry`) && method === "POST") return options.onRetry?.({ url, init }) ?? ok({ ...current, state: "paying", version: JSON.parse(String(init?.body)).version + 1, last_error_code: null });
    if (url.endsWith(`/commission-payments/${paymentId}/approve`) && method === "POST") return ok({ ...current, state: "paying", version: JSON.parse(String(init?.body)).version + 1, last_error_code: null });
    return failure(404);
  });
  vi.stubGlobal("fetch", fetcher);
  return { calls, fetcher, get current() { return current; }, set current(value: CommissionPayment) { current = value; }, get policy() { return policyValue; } };
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); PaymentState.clearAllCommissionPaymentIntents(); });

describe("CommissionPaymentsManagement", () => {
  it("denies unscoped users without issuing financial reads", async () => {
    const server = installFetch();
    const mounted = mount({ ...baseAccount, permissions_by_brand: { [brand]: [] } }); await flush();
    expect(textOf(mounted.container)).toContain("没有此品牌的佣金查看权限");
    expect(server.calls).toHaveLength(0);
    mounted.app.unmount();
  });

  it("loads the real policy and payout jobs and never infers paid from calculation state", async () => {
    const server = installFetch(); const mounted = mount(baseAccount); await flush();
    expect(textOf(mounted.container)).toContain("真实派发已关闭");
    expect(textOf(mounted.container)).toContain("派发任务");
    expect(textOf(mounted.container)).toContain("失败");
    expect(server.calls.some((call) => call.url.includes("/commission-payments?limit=20&offset=0"))).toBe(true);
    expect(byTestId(mounted.container, "commission-payment-receipt")).toBeNull();
    mounted.app.unmount();
  });

  it("requires a double review for enabling, warns about historical automatic credits, and permits paused-brand finance operations", async () => {
    const server = installFetch(); const mounted = mount(baseAccount, brand, "paused"); await flush();
    setModel(byTestId(mounted.container, "commission-payment-policy-enabled"), true); await flush();
    setModel(byTestId(mounted.container, "commission-payment-policy-reason"), "Enable after finance review"); await flush();
    click(byTestId(mounted.container, "commission-payment-policy-review")); await flush();
    expect(textOf(mounted.container)).toContain("历史 ready 且未派发周期");
    expect(textOf(mounted.container)).toContain("无需逐任务审批");
    const review = byTestId(mounted.container, "commission-payment-review");
    const labels = review!.children.filter((node) => node.tag === "dl").flatMap((dl) => dl.children.map((child) => child.tag));
    expect(labels.filter((tag) => tag === "dt")).toHaveLength(labels.filter((tag) => tag === "dd").length);
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    const put = server.calls.find((call) => call.init?.method === "PUT");
    expect(put).toBeDefined();
    expect(JSON.parse(String(put!.init?.body))).toEqual({ version: 1, enabled: true, reason: "Enable after finance review" });
    mounted.app.unmount();
  });

  it("allows failed partial-payment retry so the backend can resume only unfinished targets", async () => {
    const server = installFetch({ current: payment({ paid_count: "1", paid_points: "5" }) }); const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    setModel(byTestId(mounted.container, "commission-payment-action-reason"), "Resume only unfinished targets"); await flush();
    expect(byTestId(mounted.container, "commission-payment-retry")?.props.disabled).toBe(false);
    expect(textOf(mounted.container)).toContain("续跑未完成目标");
    mounted.app.unmount();
  });

  it("reviews and retries a failed mixed cycle with its reason, actor, version, and mode intact", async () => {
    const server = installFetch({ current: payment({ state: "failed", payout_mode: "mixed", version: 7, paid_count: "1", paid_points: "5", last_error_code: "POST_FAILED" }) });
    const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    expect(textOf(mounted.container)).toContain("混合（整周期人工审核）");
    setModel(byTestId(mounted.container, "commission-payment-action-reason"), "Resume approved mixed cycle"); await flush();
    expect(byTestId(mounted.container, "commission-payment-retry")?.props.disabled).toBe(false);
    click(byTestId(mounted.container, "commission-payment-retry")); await flush();
    const review = byTestId(mounted.container, "commission-payment-review")!;
    expect(textOf(review)).toContain("retry");
    expect(textOf(review)).toContain("v7");
    expect(textOf(review)).toContain("Resume approved mixed cycle");
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    const retries = server.calls.filter((call) => call.url.endsWith(`/commission-payments/${paymentId}/retry`));
    expect(retries).toHaveLength(1);
    expect(JSON.parse(String(retries[0].init?.body))).toEqual({ version: 7, reason: "Resume approved mixed cycle" });
    expect(new Headers(retries[0].init?.headers).get("X-Commission-Payment-Actor-ID")).toBe(actor);
    expect(textOf(mounted.container)).toContain("混合（整周期人工审核）");
    expect(textOf(mounted.container)).toContain("操作回执");
    mounted.app.unmount();
  });

  it("reviews a mixed awaiting-approval cycle through the whole-cycle approval flow", async () => {
    const snapshot = payment({ state: "awaiting_approval", payout_mode: "mixed", last_error_code: null });
    const server = installFetch({ current: snapshot }); const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    expect(textOf(mounted.container)).toContain("混合（整周期人工审核）");
    expect(textOf(mounted.container)).toContain("整个混合周期");
    setModel(byTestId(mounted.container, "commission-payment-action-reason"), "Approve the entire mixed cycle"); await flush();
    expect(byTestId(mounted.container, "commission-payment-approve")?.props.disabled).toBe(false);
    click(byTestId(mounted.container, "commission-payment-approve")); await flush();
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    const approvals = server.calls.filter((call) => call.url.endsWith(`/commission-payments/${paymentId}/approve`));
    expect(approvals).toHaveLength(1);
    expect(JSON.parse(String(approvals[0].init?.body))).toEqual({ version: 2, reason: "Approve the entire mixed cycle" });
    expect(new Headers(approvals[0].init?.headers).get("X-Commission-Payment-Actor-ID")).toBe(actor);
    expect(textOf(mounted.container)).toContain("操作回执");
    mounted.app.unmount();
  });

  it("does not offer approval for blocked payment records", async () => {
    const blocked = [
      payment({ state: "blocked", payout_mode: "mixed", last_error_code: "COMMISSION_PAYMENT_CORRECTION_REQUIRED" }),
      payment({ state: "blocked", payout_mode: "mixed", paid_count: "1", paid_points: "5", last_error_code: "COMMISSION_PAYMENT_CORRECTION_REQUIRED" }),
    ];
    for (const snapshot of blocked) {
      const server = installFetch({ current: snapshot }); const mounted = mount(baseAccount); await flush();
      click(button(mounted.container, paymentId)); await flush();
      expect(byTestId(mounted.container, "commission-payment-approve")).toBeNull();
      expect(server.calls.filter((call) => call.init?.method === "POST")).toHaveLength(0);
      mounted.app.unmount();
    }
  });

  it("keeps a stale deferred detail from replacing a newer list and detail snapshot", async () => {
    const oldDetail = deferred<Response>(); const server = installFetch({ delayFirstDetail: oldDetail }); const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    server.current = payment({ state: "paying", version: 5, last_error_code: null });
    click(byTestId(mounted.container, "commission-payments-refresh")); await flush();
    expect(textOf(mounted.container)).toContain("v5");
    oldDetail.resolve(ok(payment({ version: 2 }))); await flush();
    expect(textOf(mounted.container)).toContain("v5");
    expect(textOf(mounted.container)).toContain("派发中");
    mounted.app.unmount();
  });

  it("restores unknown intent across remount and replays its old version, body, and key only after a second review", async () => {
    let attempts = 0; const server = installFetch({ onRetry: ({ init }) => {
      attempts++;
      if (attempts === 1) throw new Error("response lost after commit");
      return ok(payment({ state: "paying", version: 3, last_error_code: null }));
    } });
    let mounted = mount(baseAccount); await flush(); click(button(mounted.container, paymentId)); await flush();
    setModel(byTestId(mounted.container, "commission-payment-action-reason"), "Retry committed payout"); await flush();
    click(byTestId(mounted.container, "commission-payment-retry")); await flush();
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    expect(textOf(mounted.container)).toContain("结果未知");
    mounted.app.unmount();

    server.current = payment({ state: "paid", version: 4, paid_count: "2", paid_points: "10", last_error_code: null });
    mounted = mount(baseAccount); await flush();
    expect(textOf(mounted.container)).toContain("待确认请求");
    click(button(mounted.container, "使用原请求重试")); await flush();
    expect(textOf(mounted.container)).toContain("Retry committed payout");
    const keyBefore = textOf(byTestId(mounted.container, "commission-payment-review-key")!);
    expect(keyBefore).toBeTruthy();
    click(button(byTestId(mounted.container, "commission-payment-review")!, "取消")); await flush();
    expect(button(mounted.container, "使用原请求重试")?.props.disabled).toBe(false);
    click(button(mounted.container, "使用原请求重试")); await flush();
    expect(textOf(byTestId(mounted.container, "commission-payment-review-key")!)).toBe(keyBefore);
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    const retries = server.calls.filter((call) => call.url.endsWith(`/commission-payments/${paymentId}/retry`));
    expect(retries).toHaveLength(2);
    expect(retries.map((call) => call.init?.body)).toEqual(['{"version":2,"reason":"Retry committed payout"}', '{"version":2,"reason":"Retry committed payout"}']);
    expect(retries.map((call) => new Headers(call.init?.headers).get("Idempotency-Key"))).toEqual([expect.any(String), expect.any(String)]);
    expect(new Headers(retries[0].init?.headers).get("Idempotency-Key")).toBe(new Headers(retries[1].init?.headers).get("Idempotency-Key"));
    expect(textOf(mounted.container)).toContain("操作回执");
    expect(textOf(mounted.container)).toContain("已入账");
    mounted.app.unmount();
  });

  it("blocks an unknown request after permission loss and restores only manual review when authority returns", async () => {
    const server = installFetch({ onRetry: () => { throw new Error("outcome unknown"); } }); const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    setModel(byTestId(mounted.container, "commission-payment-action-reason"), "Do not replay after revocation"); await flush();
    click(byTestId(mounted.container, "commission-payment-retry")); await flush();
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    expect(textOf(mounted.container)).toContain("结果未知");
    mounted.scope.value = { ...mounted.scope.value, account: { ...baseAccount, permissions_by_brand: { [brand]: ["commission.view.brand"] } } };
    await flush();
    expect(button(mounted.container, "使用原请求重试")?.props.disabled).toBe(true);
    mounted.scope.value = { ...mounted.scope.value, account: baseAccount };
    await flush();
    expect(button(mounted.container, "使用原请求重试")?.props.disabled).toBe(false);
    expect(server.calls.filter((call) => call.url.endsWith(`/commission-payments/${paymentId}/retry`))).toHaveLength(1);
    mounted.app.unmount();
  });

  it("holds a 409 intent until manual refresh, explicit discard, and a new review", async () => {
    let puts = 0; const server = installFetch({ onPolicyPut: () => { puts++; return failure(409); } }); const mounted = mount(baseAccount); await flush();
    setModel(byTestId(mounted.container, "commission-payment-policy-enabled"), true); await flush();
    setModel(byTestId(mounted.container, "commission-payment-policy-reason"), "Resolve version conflict"); await flush();
    click(byTestId(mounted.container, "commission-payment-policy-review")); await flush();
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    expect(textOf(mounted.container)).toContain("409");
    expect(byTestId(mounted.container, "commission-payment-conflict")).not.toBeNull();
    click(button(mounted.container, "手动刷新当前状态")); await flush();
    const reviewCheckbox = byTestId(mounted.container, "commission-payment-conflict")!.children.find((node) => node.tag === "label")!.children[0];
    setModel(reviewCheckbox, true); await flush();
    click(button(mounted.container, "确认丢弃并允许重新审核")); await flush();
    expect(textOf(mounted.container)).toContain("确认丢弃旧请求");
    expect(puts).toBe(1);
    mounted.app.unmount();
  });

  it("does not let delayed conflict refresh unlock discard after a newer ordinary refresh", async () => {
    const slow = deferred<Response>();
    const server = installFetch({onPolicyPut: () => failure(409)});
    const mounted = mount(baseAccount); await flush();
    setModel(byTestId(mounted.container, "commission-payment-policy-enabled"), true);
    setModel(byTestId(mounted.container, "commission-payment-policy-reason"), "Review conflict evidence"); await flush();
    click(byTestId(mounted.container, "commission-payment-policy-review")); await flush();
    setModel(byTestId(mounted.container, "commission-payment-review-confirmed"), true); await flush();
    click(byTestId(mounted.container, "commission-payment-confirm-submit")); await flush();
    server.fetcher.mockImplementationOnce(() => slow.promise);
    click(button(mounted.container, "手动刷新当前状态")); await flush();
    click(byTestId(mounted.container, "commission-payments-refresh")); await flush();
    slow.resolve(ok(policy())); await flush();
    expect(button(mounted.container, "确认丢弃并允许重新审核")?.props.disabled).toBe(true);
    expect(textOf(mounted.container)).toContain("409 原请求已冻结");
    mounted.app.unmount();
  });

  it("rejects late prior-account reads and clears visible financial authority", async () => {
    const slow = deferred<Response>();
    const server = installFetch({delayFirstDetail: slow});
    const mounted = mount(baseAccount); await flush();
    click(button(mounted.container, paymentId)); await flush();
    mounted.scope.value = {...mounted.scope.value,account:{...baseAccount,id:"88888888-8888-4888-8888-888888888888",brand_ids:[],permissions_by_brand:{}}}; await flush();
    slow.resolve(ok(payment({state:"paying",version:8,last_error_code:null}))); await flush();
    expect(textOf(mounted.container)).toContain("没有此品牌的佣金查看权限");
    expect(textOf(mounted.container)).not.toContain("v8");
    expect(byTestId(mounted.container,"commission-payment-retry")).toBeNull();
    expect(server.calls.filter(call=>call.init?.method==='POST')).toHaveLength(0);
    mounted.app.unmount();
  });
});
