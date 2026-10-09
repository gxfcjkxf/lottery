import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as ReportsApi from "./reports-api";
import * as WorkbenchApi from "./workbench-api";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";

const brandA = "11111111-1111-4111-8111-111111111111";
const brandB = "22222222-2222-4222-8222-222222222222";
const fields = ["discovery_pending_count", "discovery_failed_count", "cycle_processing_count", "cycle_waiting_count", "cycle_ready_count", "cycle_stale_count", "cycle_failed_count",
  "payment_awaiting_approval_count", "payment_processing_count", "payment_blocked_count", "payment_failed_count", "plan_processing_count", "plan_ready_count", "plan_blocked_count", "plan_failed_count",
  "execution_awaiting_approval_count", "execution_processing_count", "execution_paused_count", "execution_failed_count"] as const;
const chineseLabels = ["待发现（含未来等待）", "发现失败", "周期枚举、计算或汇总中", "周期等待中", "周期就绪（当前证据）", "周期就绪（证据过期）", "周期失败", "付款待审批", "付款处理中", "付款受阻", "付款失败", "方案处理中", "方案就绪", "方案受阻", "方案失败", "执行待审批", "执行处理中", "执行已暂停", "执行失败"];
const englishLabels = ["Discovery pending (including future checks)", "Discovery failed", "Cycles enumerating, calculating, or summarizing", "Cycles waiting", "Cycles ready (current evidence)", "Cycles ready (stale evidence)", "Cycles failed", "Payments awaiting approval", "Payments processing", "Payments blocked", "Payments failed", "Plans processing", "Plans ready", "Plans blocked", "Plans failed", "Executions awaiting approval", "Executions processing", "Executions paused", "Executions failed"];
type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
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
  const source = readFileSync(new URL("./OperationsWorkbench.vue", import.meta.url), "utf8");
  const descriptor = parse(source, { filename: "OperationsWorkbench.vue" }).descriptor;
  const compiled = compileScript(descriptor, { id: "operations-workbench-commission-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./reports-api": ReportsApi, "./workbench-api": WorkbenchApi };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => {
    if (!(specifier in modules)) throw new Error(`Unmapped component import: ${specifier}`);
    return `const {${bindings.replace(/\s+as\s+/g, ": ")}} = __modules[${JSON.stringify(specifier)}];`;
  }).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
function snapshot(brandId: string, commissionStatus: "ready" | "forbidden" = "ready") {
  const ready = <T>(data: T) => ({ status: "ready", data });
  return {
    brand_id: brandId, snapshot_at: "2026-10-07T10:00:00Z", timezone: "UTC", day_from: "2026-10-07T00:00:00Z",
    brand: ready({ name: "Example", code: "EX", state: "active" }),
    periods: ready({ pending: "0", betting: "0", closed: "0", waiting_draw: "0", drawn: "0", settling: "0", refund_pending: "0", refund_failed: "0" }),
    orders: ready({ placed: "0", abnormal: "0" }), today_bets: ready({ order_count: "0", stake_points: "0", cancelled_count: "0", abnormal_count: "0" }),
    settlement: ready({ processing: "0", awaiting_approval: "0", paying: "0", failed: "0" }), recharges: ready({ pending_count: "0", pending_points: "0" }),
    ledger: ready({ entry_count: "0", net_points: "0", recharge_points: "0", prize_credit_points: "0", prize_reversal_points: "0", refund_points: "0" }),
    balances: ready({ account_count: "0", available_points: "0", frozen_points: "0", withdrawal_points: "0", total_points: "0" }),
    reconciliation: ready({ latest_job: null }),
    sources: ready({ adapter_state: "stub", configured_games: "0", enabled_api_sources: "0", enabled_dom_sources: "0", attempts_today: "0", failed_today: "0", no_data_today: "0", last_attempt_at: null }),
    withdrawals: ready({ reviewing_count: "0", reviewing_points: "0", processing_count: "0", processing_points: "0" }),
    commissions: commissionStatus === "ready" ? { status: "ready", data: Object.fromEntries(fields.map((field, index) => [field, index === 0 ? "900719925474099312345" : String(index)])) } : { status: "forbidden", data: null },
    rewards: ready({ granted_count: "0", pending_count: "0", revoked_count: "0" }),
  };
}
function response(data: unknown) { return new Response(JSON.stringify({ success: true, data }), { status: 200, headers: { "Content-Type": "application/json" } }); }
function account(grants: string[] = ["commission.view.brand"]): AdminAccount { return { id: "33333333-3333-4333-8333-333333333333", super_admin: false, brand_ids: [brandA, brandB], permissions: [], permissions_by_brand: { [brandA]: grants, [brandB]: grants } }; }
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
function find(node: HostNode, predicate: (candidate: HostNode) => boolean): HostNode | null { if (predicate(node)) return node; for (const child of node.children) { const found = find(child, predicate); if (found) return found; } return null; }
function buttons(node: HostNode): HostNode[] { return (node.tag === "button" ? [node] : []).concat(...node.children.map(buttons)); }
async function flush() { for (let i = 0; i < 12; i++) await Promise.resolve(); await nextTick(); }
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
function mount(initialAccount: AdminAccount, initialBrand: string) {
  const scope = ref({ account: initialAccount, brandId: initialBrand }), container = element("root"), context = createAdminI18n();
  const emitted: string[] = [];
  const component = compileComponent();
  const app = renderer.createApp({ setup: () => () => h(component, { ...scope.value,
    onSessionInvalid: () => emitted.push("session-invalid"), onNavigate: (destination: string) => emitted.push(destination) }) });
  app.provide(adminI18nKey, context); app.mount(container);
  return { app, scope, container, context, emitted };
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("OperationsWorkbench commissions", () => {
  it("renders all localized workflow counters, a permission-gated link, and no payment controls", async () => {
    const read = vi.fn<typeof fetch>().mockResolvedValue(response(snapshot(brandA)));
    vi.stubGlobal("fetch", read);
    const mounted = mount(account(), brandA); await flush();
    expect(read).toHaveBeenCalledOnce();
    expect(new Headers(read.mock.calls[0][1]?.headers).get("X-Brand-ID")).toBe(brandA);
    const chinese = textOf(mounted.container);
    expect(chinese).toContain("佣金");
    expect(chinese).toContain("900719925474099312345");
    chineseLabels.forEach((label) => expect(chinese).toContain(label));
    expect(chinese).toContain("不代表完整财务证据或审批");
    fields.slice(1).forEach((_field, index) => expect(chinese).toContain(String(index + 1)));
    expect(chinese).toContain("不代表资金、付款授权或执行授权");
    expect(buttons(mounted.container).some((button) => /付款|派发|支付|Pay|Payout/i.test(textOf(button)))).toBe(false);
    const open = buttons(mounted.container).find((button) => textOf(button).includes("打开管理页面") && button.props.disabled !== true);
    expect(open).toBeDefined();
    (open!.props.onClick as () => void)();
    expect(mounted.emitted).toContain("佣金和奖励");

    mounted.context.setLocale("en"); await nextTick();
    const english = textOf(mounted.container);
    expect(english).toContain("Commissions");
    englishLabels.forEach((label) => expect(english).toContain(label));
    expect(english).toContain("it does not mean complete financial evidence or approval");
    expect(english).toContain("workflow indicators, not funds or payment/execution authorization");
    mounted.app.unmount();
  });

  it("does not display a late ready response after permission revocation", async () => {
    const delayed = deferred<Response>();
    const read = vi.fn<typeof fetch>().mockReturnValueOnce(delayed.promise).mockResolvedValueOnce(response(snapshot(brandA, "forbidden")));
    vi.stubGlobal("fetch", read);
    const mounted = mount(account(), brandA); await flush();
    expect(read).toHaveBeenCalledOnce();
    mounted.scope.value = { account: account([]), brandId: brandA };
    await flush();
    expect(read).toHaveBeenCalledTimes(2);
    expect(textOf(mounted.container)).toContain("当前账号没有此模块的查看权限");
    delayed.resolve(response(snapshot(brandA, "ready")));
    await flush();
    expect(textOf(mounted.container)).not.toContain("900719925474099312345");
    expect(textOf(mounted.container)).toContain("当前账号没有此模块的查看权限");
    mounted.app.unmount();
  });

  it("drops a previous brand snapshot while the newly selected brand is loading", async () => {
    const delayed = deferred<Response>();
    const read = vi.fn<typeof fetch>().mockReturnValueOnce(Promise.resolve(response(snapshot(brandA)))).mockReturnValueOnce(delayed.promise);
    vi.stubGlobal("fetch", read);
    const mounted = mount(account(), brandA); await flush();
    expect(textOf(mounted.container)).toContain("900719925474099312345");
    mounted.scope.value = { account: account(), brandId: brandB };
    await flush();
    expect(textOf(mounted.container)).not.toContain("900719925474099312345");
    expect(textOf(mounted.container)).toContain("正在读取当前品牌快照");
    delayed.resolve(response(snapshot(brandB, "forbidden")));
    await flush();
    expect(textOf(mounted.container)).not.toContain("900719925474099312345");
    mounted.app.unmount();
  });
});
