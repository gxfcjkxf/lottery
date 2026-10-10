import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import { AdminApiError, type AdminAccount } from "./admin-api";
import * as CommissionCyclesApi from "./commission-cycles-api";
import * as CommissionCyclesState from "./commission-cycles-state";

const brandA = "11111111-1111-4111-8111-111111111111";
const brandB = "22222222-2222-4222-8222-222222222222";
const actorA = "33333333-3333-4333-8333-333333333333";
const actorB = "44444444-4444-4444-8444-444444444444";
const cycleId = "55555555-5555-4555-8555-555555555555";
const anchorId = "66666666-6666-4666-8666-666666666666";
const runA = "77777777-7777-4777-8777-777777777777";
const runB = "88888888-8888-4888-8888-888888888888";
const discoveryId = "99999999-9999-4999-8999-999999999999";
const timestamp = "2026-10-06T00:00:00Z";
const baseAccount: AdminAccount = {
  id: actorA, super_admin: false, brand_ids: [brandA, brandB], permissions: [],
  permissions_by_brand: {
    [brandA]: ["commission.view.brand", "commission.run.brand", "commission.retry.brand"],
    [brandB]: ["commission.view.brand", "commission.run.brand", "commission.retry.brand"],
  },
};

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode {
  return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } };
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

function compileComponent() {
  const source = readFileSync(new URL("./CommissionCyclesManagement.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "CommissionCyclesManagement.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "commission-cycles-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commission-cycles-api": CommissionCyclesApi, "./commission-cycles-state": CommissionCyclesState };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => {
    if (!(specifier in modules)) throw new Error(`Unmapped component import: ${specifier}`);
    return `const {${bindings.replace(/\s+as\s+/g, ": ")}} = __modules[${JSON.stringify(specifier)}];`;
  }).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}

function cycle(brandId: string, evidenceCurrent = true, state: "ready" | "failed" | "enumerating" = "ready", version = 2) {
  return {
    id: cycleId, brand_id: brandId, window_from: "2026-10-01T00:00:00Z", window_to: "2026-10-08T00:00:00Z", anchor_order_id: anchorId,
    calendar: { timezone: "UTC", cycle: "weekly", boundary_time: "00:00:00", weekday: 1, month_day: null, short_month: "" },
    state, version, target_count: "3", scan_complete: true, current_run_id: runA, current_generation: "9007199254740999", evidence_epoch: "4",
    evidence_current: evidenceCurrent, calculated_count: "3", earning_count: "2", total_points: "9007199254740992", created_by: actorA,
    creation_actor_type: "admin", reason: "Monthly review", created_at: timestamp, updated_at: timestamp, last_error_code: state === "failed" ? "CALCULATION_FAILED" : null,
    creation_audit_log_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  };
}
function run(brandId: string, id: string, generation: string) {
  return { id, brand_id: brandId, cycle_id: cycleId, generation, evidence_epoch: "4", state: "ready", calculated_count: "1", earning_count: "1", total_points: "10000000000000001", created_at: timestamp };
}
function discovery(brandId: string, state: "pending" | "registered" | "failed" = "failed") {
  return { id: discoveryId, brand_id: brandId, state, version: 4, cycle_id: state === "registered" ? cycleId : null, window_from: null, window_to: null, next_check_at: timestamp, last_error_code: state === "failed" ? "ORDER_NOT_READY" : null, last_audit_log_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", created_at: timestamp, updated_at: timestamp };
}
function page(brandId: string, items: unknown[], offset = 0) { return { brand_id: brandId, items, total_count: String(items.length), limit: 20, offset }; }
function ok(data: unknown, status = 200) { return new Response(JSON.stringify({ success: true, data }), { status }); }
function error(status: number, message: string) { return new Response(JSON.stringify({ success: false, error: { code: status === 401 ? "UNAUTHORIZED" : status === 409 ? "VERSION_CONFLICT" : "REQUEST_FAILED", message } }), { status }); }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (cause: unknown) => void; const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; }); return { promise, resolve, reject }; }
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function findNode(node: HostNode, match: (candidate: HostNode) => boolean): HostNode | null { if (match(node)) return node; for (const child of node.children) { const found = findNode(child, match); if (found) return found; } return null; }
function buttonTexts(node: HostNode): string[] { return (node.tag === "button" ? [textOf(node)] : []).concat(...node.children.map(buttonTexts)); }
async function flush() { for (let i = 0; i < 10; i++) await Promise.resolve(); await nextTick(); }
function mount(account: AdminAccount, brandId: string) {
  const scope = ref({ account, brandId }), container = element("root"), emitted: string[] = [];
  const component = compileComponent();
  const app = renderer.createApp({ setup: () => () => h(component, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, createAdminI18n()); app.mount(container);
  return { app, container, scope, emitted };
}
function defaultFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input), brandId = new Headers(init?.headers).get("X-Brand-ID") ?? brandA;
  if (url.includes("/commission-discovery?")) return Promise.resolve(ok(page(brandId, [discovery(brandId)])));
  if (url.endsWith("/commission-cycles?limit=20&offset=0")) return Promise.resolve(ok(page(brandId, [cycle(brandId, false)])));
  if (url.endsWith(`/commission-cycles/${cycleId}/earnings?limit=20&offset=0`)) return Promise.resolve(ok({ ...page(brandId, []), cycle_id: cycleId }));
  if (url.endsWith(`/commission-cycles/${cycleId}/runs?limit=20&offset=0`)) return Promise.resolve(ok({ ...page(brandId, [run(brandId, runA, "9007199254740999"), run(brandId, runB, "9007199254741000")]), cycle_id: cycleId }));
  if (url.includes(`/commission-cycles/${cycleId}/runs/${runA}/earnings?`) || url.includes(`/commission-cycles/${cycleId}/runs/${runB}/earnings?`)) return Promise.resolve(ok({ ...page(brandId, []), cycle_id: cycleId, run_id: url.includes(runA) ? runA : runB }));
  if (url.includes(`/commission-cycles/${cycleId}/runs/${runA}/allocations?`) || url.includes(`/commission-cycles/${cycleId}/runs/${runB}/allocations?`)) return Promise.resolve(ok({ ...page(brandId, []), cycle_id: cycleId, run_id: url.includes(runA) ? runA : runB, agent_id: null, order_id: null }));
  if (url.includes(`/commission-cycles/${cycleId}/runs/${runA}/calculations?`)) return Promise.resolve(ok({ ...page(brandId, []), cycle_id: cycleId, run_id: runA }));
  if (url.includes(`/commission-cycles/${cycleId}/runs/${runB}/calculations?`)) return Promise.resolve(ok({ ...page(brandId, []), cycle_id: cycleId, run_id: runB }));
  if (url.endsWith(`/commission-cycles/${cycleId}`)) return Promise.resolve(ok(cycle(brandId, false)));
  return Promise.resolve(error(404, "Unknown test request"));
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); CommissionCyclesState.clearAllPendingCommissionCycleWrites(); });

describe("CommissionCyclesManagement", () => {
  it("labels stale ready evidence, reads calculations for the explicitly selected run, and shows discovery state", async () => {
    const reads = {
      list: vi.fn(async (brand: string) => ({ brand_id: brand, items: [cycle(brand, false) as unknown as CommissionCyclesApi.CommissionCycleRecord], total_count: "1", limit: 20, offset: 0 })),
      read: vi.fn(async (brand: string) => cycle(brand, false) as unknown as CommissionCyclesApi.CommissionCycleRecord),
      earnings: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [], total_count: "0", limit: 20, offset: 0 })),
      runEarnings: vi.fn(async (brand: string, id: string, runId: string, limit = 20, offset = 0) => ({ brand_id: brand, cycle_id: id, run_id: runId,
        items: [{ id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab", brand_id: brand, cycle_id: id, run_id: runId, agent_id: actorA, member_id: actorB, exact_amount: { numerator: "3", denominator: "2" }, points: "2", created_at: timestamp }], total_count: "1", limit, offset })),
      allocations: vi.fn(async (brand: string, id: string, runId: string, query: CommissionCyclesApi.CommissionAllocationQuery = {}) => ({ brand_id: brand, cycle_id: id, run_id: runId, agent_id: query.agent_id ?? null, order_id: query.order_id ?? null, items: [], total_count: "0", limit: query.limit ?? 20, offset: query.offset ?? 0 })),
      runs: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [run(brand, runA, "9007199254740999"), run(brand, runB, "9007199254741000")] as unknown as CommissionCyclesApi.CommissionRun[], total_count: "2", limit: 20, offset: 0 })),
      calculations: vi.fn(async (brand: string, id: string, runId: string) => ({ brand_id: brand, cycle_id: id, run_id: runId, items: [], total_count: "0", limit: 20, offset: 0 })),
      discoveries: vi.fn(async (brand: string) => ({ brand_id: brand, items: [discovery(brand)], total_count: "1", limit: 20, offset: 0 })),
    };
    vi.spyOn(CommissionCyclesApi, "createCommissionCyclesApi").mockReturnValue(reads as unknown as CommissionCyclesApi.CommissionCyclesApi);
    const mounted = mount(baseAccount, brandA); await flush();
    expect(textOf(mounted.container)).toContain("证据已过期");
    expect(textOf(mounted.container)).toContain("ORDER_NOT_READY");
    expect(buttonTexts(mounted.container).some((label) => /确认审核|确认派发|Approve commission|Pay out/i.test(label))).toBe(false);
    const cycleRow = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes(cycleId));
    (cycleRow!.props.onClick as () => void)(); await flush();
    expect(textOf(mounted.container)).toContain("核算就绪");
    const secondRun = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes(runB));
    (secondRun!.props.onClick as () => void)(); await flush();
    expect(reads.calculations).toHaveBeenCalledWith(brandA, cycleId, runB, 20, 0);
    expect(reads.runEarnings).toHaveBeenCalledWith(brandA, cycleId, runB, 20, 0);
    expect(reads.allocations).toHaveBeenCalledWith(brandA, cycleId, runB, { limit: 20, offset: 0 });
    expect(textOf(mounted.container)).toContain("未舍入分配明细");
    expect(textOf(mounted.container)).toContain("每位代理的整周期收入只舍入一次");
    expect(textOf(mounted.container)).toContain("整周期舍入积分 2");
    expect(textOf(mounted.container)).toContain("整周期精确合计 3/2");
    expect(textOf(mounted.container)).toContain("9007199254740999");
    mounted.app.unmount();
  });

  it("filters allocation rows only for the explicitly selected run", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const delayed = deferred<CommissionCyclesApi.CommissionAllocationPage>(); let holdRunB = true;
    const filteredRows = ["loss", "turnover"].map((mode, index) => ({ brand_id: brandA, cycle_id: cycleId, run_id: runB,
      calculation_id: index === 0 ? "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab" : "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      order_id: index === 0 ? anchorId : cycleId, agent_id: actorA, member_id: actorB, bettor_member_id: actorB,
      base_points: "100", mode, agent_ratio: "0.3", downstream_ratio: "0.1", difference_ratio: "0.2",
      exact_amount: { numerator: "20", denominator: "1" }, created_at: timestamp }));
    const reads = {
      list: vi.fn(async (brand: string) => ({ brand_id: brand, items: [cycle(brand, true) as unknown as CommissionCyclesApi.CommissionCycleRecord], total_count: "1", limit: 20, offset: 0 })),
      read: vi.fn(async (brand: string) => cycle(brand, true) as unknown as CommissionCyclesApi.CommissionCycleRecord),
      earnings: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [], total_count: "0", limit: 20, offset: 0 })),
      runEarnings: vi.fn(async (brand: string, id: string, runId: string, limit = 20, offset = 0) => ({ brand_id: brand, cycle_id: id, run_id: runId, items: [], total_count: "0", limit, offset })),
      allocations: vi.fn((brand: string, id: string, runId: string, query: CommissionCyclesApi.CommissionAllocationQuery = {}) => {
        if (runId === runB && !query.agent_id && holdRunB) { holdRunB = false; return delayed.promise; }
        const items = query.agent_id ? filteredRows : [];
        return Promise.resolve({ brand_id: brand, cycle_id: id, run_id: runId, agent_id: query.agent_id ?? null, order_id: query.order_id ?? null, items, total_count: String(items.length), limit: query.limit ?? 20, offset: query.offset ?? 0 });
      }),
      runs: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [run(brand, runA, "1"), run(brand, runB, "2")] as unknown as CommissionCyclesApi.CommissionRun[], total_count: "2", limit: 20, offset: 0 })),
      calculations: vi.fn(async (brand: string, id: string, runId: string) => ({ brand_id: brand, cycle_id: id, run_id: runId, items: [], total_count: "0", limit: 20, offset: 0 })),
      discoveries: vi.fn(async (brand: string) => ({ brand_id: brand, items: [], total_count: "0", limit: 20, offset: 0 })),
    };
    vi.spyOn(CommissionCyclesApi, "createCommissionCyclesApi").mockReturnValue(reads as unknown as CommissionCyclesApi.CommissionCyclesApi);
    const mounted = mount(baseAccount, brandA); await flush();
    const cycleRow = findNode(mounted.container, (node) => node.tag === "button" && node.props["data-cycle-id"] === cycleId);
    (cycleRow!.props.onClick as () => void)(); await flush();
    const secondRun = findNode(mounted.container, (node) => node.tag === "button" && node.props["data-run-id"] === runB);
    (secondRun!.props.onClick as () => void)(); await flush();
    const agentInput = findNode(mounted.container, (node) => node.tag === "input" && node.props["aria-label"] === "代理编号筛选");
    const updateAgent = agentInput!.props["onUpdate:modelValue"] as ((value: string) => void) | Array<(value: string) => void>;
    for (const update of Array.isArray(updateAgent) ? updateAgent : [updateAgent]) update(actorA); await nextTick();
    delayed.resolve({ brand_id: brandA, cycle_id: cycleId, run_id: runB, agent_id: null, order_id: null,
      items: [{ brand_id: brandA, cycle_id: cycleId, run_id: runB, calculation_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", order_id: anchorId, agent_id: actorB, member_id: actorA, bettor_member_id: actorB, base_points: "100", mode: "loss", agent_ratio: "0.3", downstream_ratio: "0.1", difference_ratio: "0.2", exact_amount: { numerator: "20", denominator: "1" }, created_at: timestamp }], total_count: "1", limit: 20, offset: 0 });
    await flush(); expect(textOf(mounted.container)).not.toContain("dddddddd-dddd-4ddd-8ddd-dddddddddddd");
    const filter = findNode(mounted.container, (node) => node.tag === "button" && textOf(node) === "筛选");
    (filter!.props.onClick as () => void)(); await flush();
    expect(reads.allocations).toHaveBeenLastCalledWith(brandA, cycleId, runB, { limit: 20, offset: 0, agent_id: actorA });
    expect(textOf(mounted.container)).toContain("输赢"); expect(textOf(mounted.container)).toContain("流水");
    expect(reads.allocations.mock.calls.some(([, , runId]) => runId !== runB)).toBe(true); // Initial history reads are bound to the previously selected run.
    mounted.app.unmount();
  });

  it("ignores late 401 allocation reads after a run switch and after an allocation filter edit", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const oldRunRead = deferred<CommissionCyclesApi.CommissionAllocationPage>();
    const oldFilterRead = deferred<CommissionCyclesApi.CommissionAllocationPage>();
    let holdRunA = true;
    const emptyAllocationPage = (brand: string, id: string, runId: string, query: CommissionCyclesApi.CommissionAllocationQuery = {}) => ({ brand_id: brand, cycle_id: id, run_id: runId, agent_id: query.agent_id ?? null, order_id: query.order_id ?? null, items: [], total_count: "0", limit: query.limit ?? 20, offset: query.offset ?? 0 });
    const reads = {
      list: vi.fn(async (brand: string) => ({ brand_id: brand, items: [cycle(brand, true) as unknown as CommissionCyclesApi.CommissionCycleRecord], total_count: "1", limit: 20, offset: 0 })),
      read: vi.fn(async (brand: string) => cycle(brand, true) as unknown as CommissionCyclesApi.CommissionCycleRecord),
      earnings: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [], total_count: "0", limit: 20, offset: 0 })),
      runEarnings: vi.fn(async (brand: string, id: string, runId: string, limit = 20, offset = 0) => ({ brand_id: brand, cycle_id: id, run_id: runId, items: [], total_count: "0", limit, offset })),
      allocations: vi.fn((brand: string, id: string, runId: string, query: CommissionCyclesApi.CommissionAllocationQuery = {}) => {
        if (runId === runA && holdRunA) { holdRunA = false; return oldRunRead.promise; }
        if (query.agent_id === actorA) return oldFilterRead.promise;
        return Promise.resolve(emptyAllocationPage(brand, id, runId, query));
      }),
      runs: vi.fn(async (brand: string) => ({ brand_id: brand, cycle_id: cycleId, items: [run(brand, runA, "1"), run(brand, runB, "2")] as unknown as CommissionCyclesApi.CommissionRun[], total_count: "2", limit: 20, offset: 0 })),
      calculations: vi.fn(async (brand: string, id: string, runId: string) => ({ brand_id: brand, cycle_id: id, run_id: runId, items: [], total_count: "0", limit: 20, offset: 0 })),
      discoveries: vi.fn(async (brand: string) => ({ brand_id: brand, items: [], total_count: "0", limit: 20, offset: 0 })),
    };
    vi.spyOn(CommissionCyclesApi, "createCommissionCyclesApi").mockReturnValue(reads as unknown as CommissionCyclesApi.CommissionCyclesApi);
    const mounted = mount(baseAccount, brandA); await flush();
    const cycleRow = findNode(mounted.container, (node) => node.tag === "button" && node.props["data-cycle-id"] === cycleId);
    (cycleRow!.props.onClick as () => void)(); await flush();
    const secondRun = findNode(mounted.container, (node) => node.tag === "button" && node.props["data-run-id"] === runB);
    (secondRun!.props.onClick as () => void)(); await flush();
    oldRunRead.reject(new AdminApiError("expired old run", 401)); await flush();
    const agentInput = findNode(mounted.container, (node) => node.tag === "input" && node.props["aria-label"] === "代理编号筛选");
    const updateAgent = agentInput!.props["onUpdate:modelValue"] as ((value: string) => void) | Array<(value: string) => void>;
    const update = (value: string) => { for (const setter of Array.isArray(updateAgent) ? updateAgent : [updateAgent]) setter(value); };
    update(actorA); await nextTick();
    const filter = findNode(mounted.container, (node) => node.tag === "button" && textOf(node) === "筛选");
    (filter!.props.onClick as () => void)(); await flush();
    update(actorB); await nextTick();
    oldFilterRead.reject(new AdminApiError("expired old filter", 401)); await flush();
    expect(mounted.emitted).toEqual([]);
    expect(textOf(mounted.container)).not.toContain("expired old");
    mounted.app.unmount();
  });

  it("retains an unknown create body and key across unmount and replays that exact request", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    let firstWrite = true;
    const requests: Array<{ body: string; key: string }> = [];
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      const url = String(input), method = init?.method ?? "GET", brandId = new Headers(init?.headers).get("X-Brand-ID") ?? brandA;
      if (method === "POST" && url.endsWith("/commission-cycles")) {
        requests.push({ body: String(init?.body), key: new Headers(init?.headers).get("Idempotency-Key") ?? "" });
        if (firstWrite) { firstWrite = false; return Promise.resolve(error(503, "temporarily unavailable")); }
        return Promise.resolve(ok({ ...cycle(brandId, false, "enumerating", 1), anchor_order_id: anchorId, reason: "Checked anchor", creation_actor_type: "admin", created_by: actorA }, 201));
      }
      return defaultFetch(input, init);
    }));
    const first = mount(baseAccount, brandA); await flush();
    const anchor = findNode(first.container, (node) => node.tag === "input");
    (anchor!.props["onUpdate:modelValue"] as (value: string) => void)(anchorId);
    const reason = findNode(first.container, (node) => node.tag === "textarea");
    (reason!.props["onUpdate:modelValue"] as (value: string) => void)("Checked anchor"); await nextTick();
    const review = findNode(first.container, (node) => node.tag === "button" && textOf(node).includes("检查并确认登记"));
    (review!.props.onClick as () => void)(); await nextTick();
    const checkbox = findNode(first.container, (node) => node.tag === "input" && node.props.type === "checkbox");
    (checkbox!.props["onUpdate:modelValue"] as (value: boolean) => void)(true); await nextTick();
    const submit = findNode(first.container, (node) => node.tag === "button" && textOf(node).includes("确认并提交"));
    (submit!.props.onClick as () => void)(); await flush();
    expect(textOf(first.container)).toContain("写入结果未知");
    expect(requests).toHaveLength(1);
    first.app.unmount();

    const second = mount(baseAccount, brandA); await flush();
    expect(buttonTexts(second.container).some((label) => label.includes("丢弃此请求"))).toBe(false);
    const recover = findNode(second.container, (node) => node.tag === "button" && textOf(node).includes("检查并恢复原请求"));
    expect(recover).not.toBeNull(); (recover!.props.onClick as () => void)(); await nextTick();
    const replayCheck = findNode(second.container, (node) => node.tag === "input" && node.props.type === "checkbox");
    (replayCheck!.props["onUpdate:modelValue"] as (value: boolean) => void)(true); await nextTick();
    const replay = findNode(second.container, (node) => node.tag === "button" && textOf(node).includes("确认并提交"));
    (replay!.props.onClick as () => void)(); await flush();
    expect(requests).toHaveLength(2); expect(requests[1]).toEqual(requests[0]);
    expect(textOf(second.container)).toContain("服务器确认的原始回执");
    expect(textOf(second.container)).toContain("回执状态 / 版本");
    expect(textOf(second.container)).toContain("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa");
    second.app.unmount();
  });

  it.each(["brand", "account", "permission"] as const)("ignores a late read after %s scope change", async (change) => {
    const deferredRead = deferred<Response>(); let firstList = true;
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      if (String(input).includes("/commission-cycles?limit=20&offset=0") && firstList) { firstList = false; return deferredRead.promise; }
      return defaultFetch(input, init);
    }));
    const mounted = mount(baseAccount, brandA); await flush();
    if (change === "brand") mounted.scope.value = { account: baseAccount, brandId: brandB };
    if (change === "account") mounted.scope.value = { account: { ...baseAccount, id: actorB }, brandId: brandA };
    if (change === "permission") mounted.scope.value = { account: { ...baseAccount, permissions_by_brand: { [brandA]: [], [brandB]: [] } }, brandId: brandA };
    await flush(); deferredRead.resolve(ok(page(brandA, [cycle(brandA, true)]))); await flush();
    expect(textOf(mounted.container)).not.toContain("证据当前");
    if (change === "permission") expect(textOf(mounted.container)).toContain("没有此品牌佣金周期查看权限");
    mounted.app.unmount();
  });

  it("emits session-invalid on a current 401 read", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>(() => Promise.resolve(error(401, "session expired"))));
    const mounted = mount(baseAccount, brandA); await flush();
    expect(mounted.emitted).toContain("session-invalid");
    expect(textOf(mounted.container)).not.toContain(cycleId);
    mounted.app.unmount();
  });

  it("allows a retry-only operator to review a failed cycle without create permission", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      const url = String(input), brandId = new Headers(init?.headers).get("X-Brand-ID") ?? brandA;
      if (url.endsWith("/commission-cycles?limit=20&offset=0")) return Promise.resolve(ok(page(brandId, [cycle(brandId, true, "failed")])));
      if (url.endsWith(`/commission-cycles/${cycleId}`)) return Promise.resolve(ok(cycle(brandId, true, "failed")));
      return defaultFetch(input, init);
    }));
    const retryOnly: AdminAccount = { ...baseAccount, permissions_by_brand: { [brandA]: ["commission.view.brand", "commission.retry.brand"] } };
    const mounted = mount(retryOnly, brandA); await flush();
    const create = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("检查并确认登记"));
    expect(create?.props.disabled).toBe(true);
    const card = findNode(mounted.container, (node) => node.tag === "button" && node.props["data-cycle-id"] === cycleId);
    (card!.props.onClick as () => void)(); await flush();
    const retry = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("检查并确认重试"));
    const retryLabel = findNode(mounted.container, (node) => node.tag === "label" && textOf(node).includes("重试原因"));
    const retryField = findNode(retryLabel!, (node) => node.tag === "textarea");
    (retryField!.props["onUpdate:modelValue"] as (value: string) => void)("Retry failed cycle"); await nextTick();
    expect(retry).not.toBeNull(); expect(retry!.props.disabled).toBe(false);
    mounted.app.unmount();
  });

  it("does not emit session-invalid for a stale 401 response", async () => {
    const delayed = deferred<Response>(); let firstList = true;
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      if (String(input).includes("/commission-cycles?limit=20&offset=0") && firstList) { firstList = false; return delayed.promise; }
      return defaultFetch(input, init);
    }));
    const mounted = mount(baseAccount, brandA); await flush();
    mounted.scope.value = { account: baseAccount, brandId: brandB }; await flush();
    delayed.resolve(error(401, "expired old scope")); await flush();
    expect(mounted.emitted).toEqual([]);
    mounted.app.unmount();
  });

  it("retains a 409 intent until explicit reload, review, and discard", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      if (init?.method === "POST") return Promise.resolve(error(409, "version conflict"));
      return defaultFetch(input, init);
    }));
    const mounted = mount(baseAccount, brandA); await flush();
    const anchor = findNode(mounted.container, (node) => node.tag === "input");
    (anchor!.props["onUpdate:modelValue"] as (value: string) => void)(anchorId);
    const reason = findNode(mounted.container, (node) => node.tag === "textarea");
    (reason!.props["onUpdate:modelValue"] as (value: string) => void)("Reviewed anchor"); await nextTick();
    const reviewButton = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("检查并确认登记"));
    (reviewButton!.props.onClick as () => void)(); await nextTick();
    const check = findNode(mounted.container, (node) => node.tag === "input" && node.props.type === "checkbox");
    (check!.props["onUpdate:modelValue"] as (value: boolean) => void)(true); await nextTick();
    const submit = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("确认并提交"));
    (submit!.props.onClick as () => void)(); await flush();
    const discardBeforeReload = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("重新读取并审阅后，明确丢弃"));
    expect(discardBeforeReload).toBeNull();
    const reload = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("明确重新读取目标状态"));
    (reload!.props.onClick as () => void)(); await flush();
    const discard = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("重新读取并审阅后，明确丢弃"));
    expect(discard).not.toBeNull(); expect(discard!.props.disabled).toBe(true);
    const reviewLatest = findNode(mounted.container, (node) => node.tag === "input" && node.props.type === "checkbox");
    (reviewLatest!.props.onChange as (event: { target: { checked: boolean } }) => void)({ target: { checked: true } }); await nextTick();
    const discardEnabled = findNode(mounted.container, (node) => node.tag === "button" && textOf(node).includes("重新读取并审阅后，明确丢弃"));
    expect(discardEnabled!.props.disabled).toBe(false);
    (discardEnabled!.props.onClick as () => void)(); await flush();
    expect(CommissionCyclesState.listPendingCommissionCycleWrites(actorA, brandA)).toHaveLength(0);
    mounted.app.unmount();
  });

  it("conflict reloads cannot overlap or overwrite a newer normal refresh", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const delayed = deferred<Response>(); let conflictReload = false, lists = 0;
    const fetcher = vi.fn<typeof fetch>((input, init) => {
      if (init?.method === "POST") return Promise.resolve(error(409, "version conflict"));
      if (String(input).includes("/commission-cycles?")) {
        lists++;
        if (conflictReload && lists === 2) return delayed.promise;
        if (conflictReload) return Promise.resolve(ok(page(brandA, [{ ...cycle(brandA, false), version: 99 }])));
      }
      return defaultFetch(input, init);
    });
    vi.stubGlobal("fetch", fetcher);
    const mounted = mount(baseAccount, brandA); await flush();
    const anchor = findNode(mounted.container, node => node.tag === "input");
    (anchor!.props["onUpdate:modelValue"] as (value: string) => void)(anchorId);
    const reason = findNode(mounted.container, node => node.tag === "textarea");
    (reason!.props["onUpdate:modelValue"] as (value: string) => void)("Reviewed conflict anchor"); await nextTick();
    const review = findNode(mounted.container, node => node.tag === "button" && textOf(node).includes("检查并确认登记"));
    (review!.props.onClick as () => void)(); await nextTick();
    const check = findNode(mounted.container, node => node.tag === "input" && node.props.type === "checkbox");
    (check!.props["onUpdate:modelValue"] as (value: boolean) => void)(true); await nextTick();
    const submit = findNode(mounted.container, node => node.tag === "button" && textOf(node).includes("确认并提交"));
    (submit!.props.onClick as () => void)(); await flush();
    conflictReload = true;
    const reload = findNode(mounted.container, node => node.tag === "button" && textOf(node).includes("明确重新读取目标状态"));
    (reload!.props.onClick as () => void)(); await nextTick();
    expect(reload!.props.disabled).toBe(true);
    (reload!.props.onClick as () => void)(); await nextTick();
    expect(lists).toBe(2);
    const refresh = findNode(mounted.container, node => node.tag === "button" && textOf(node) === "刷新");
    (refresh!.props.onClick as () => void)(); await flush();
    expect(textOf(mounted.container)).toContain("版本 99");
    delayed.resolve(ok(page(brandA, [{ ...cycle(brandA, false), version: 2 }]))); await flush();
    expect(textOf(mounted.container)).toContain("版本 99");
    expect(findNode(mounted.container, node => node.tag === "button" && textOf(node).includes("重新读取并审阅后，明确丢弃"))).toBeNull();
    expect(CommissionCyclesState.listPendingCommissionCycleWrites(actorA, brandA)).toHaveLength(1);
    mounted.app.unmount();
  });

  it("clears a selected discovery when a new page no longer contains it", async () => {
    let removeDiscovery = false;
    vi.stubGlobal("fetch", vi.fn<typeof fetch>((input, init) => {
      if (removeDiscovery && String(input).includes("/commission-discovery?")) return Promise.resolve(ok(page(brandA, [])));
      return defaultFetch(input, init);
    }));
    const mounted = mount(baseAccount, brandA); await flush();
    const discovery = findNode(mounted.container, node => node.tag === "button" && textOf(node).includes(discoveryId));
    (discovery!.props.onClick as () => void)(); await flush();
    expect(textOf(mounted.container)).toContain("所选发现记录");
    removeDiscovery = true;
    const refresh = findNode(mounted.container, node => node.tag === "button" && textOf(node) === "刷新");
    (refresh!.props.onClick as () => void)(); await flush();
    expect(textOf(mounted.container)).not.toContain("所选发现记录");
    expect(findNode(mounted.container, node => node.tag === "button" && textOf(node).includes("检查并确认重试发现记录"))).toBeNull();
    mounted.app.unmount();
  });
});
