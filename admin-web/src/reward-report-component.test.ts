import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as RewardReportApi from "./reward-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", actor2 = "33333333-3333-4333-8333-333333333333", member = "44444444-4444-4444-8444-444444444444", order = "55555555-5555-4555-8555-555555555555";
const huge = "9223372036854775808123456789012345678901234567890";
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: ["reward.view.brand"], permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } };
const posting = { entry_count: "3", grant_entry_count: "2", grant_points: huge, reversal_entry_count: "1", reversal_points: `${BigInt(huge) + 7n}`, net_points: "-7" };
const orderTotals = { order_count: "3", original_points: `${BigInt(huge) + 100n}`, granted_count: "1", granted_points: huge, pending_count: "1", pending_points: "50", revoked_count: "1", revoked_points: "50" };
const grantedOnly = { order_count: "1", original_points: huge, granted_count: "1", granted_points: huge, pending_count: "0", pending_points: "0", revoked_count: "0", revoked_points: "0" };
const pendingOnly = { order_count: "1", original_points: "50", granted_count: "0", granted_points: "0", pending_count: "1", pending_points: "50", revoked_count: "0", revoked_points: "0" };
const revokedOnly = { order_count: "1", original_points: "50", granted_count: "0", granted_points: "0", pending_count: "0", pending_points: "0", revoked_count: "1", revoked_points: "50" };
type Node = { tag: string; props: Record<string, unknown>; children: Node[]; text: string; parent?: Node; value?: string; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => Node; options: Node[] };
function element(tag: string): Node { return { tag, props: {}, children: [], text: "", value: "", addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((c) => c.tag === "option"); } }; }
const renderer = createRenderer<Node, Node>({
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (n, text) => { n.text = text; }, setElementText: (n, text) => { n.text = text; n.children = []; },
  patchProp: (n, key, _old, value) => { n.props[key] = value; if (key === "value") n.value = String(value ?? ""); },
  insert: (n, p, a) => { if (n.parent) { const i = n.parent.children.indexOf(n); if (i >= 0) n.parent.children.splice(i, 1); } n.parent = p; const i = a ? p.children.indexOf(a) : -1; if (i < 0) p.children.push(n); else p.children.splice(i, 0, n); },
  remove: (n) => { if (!n.parent) return; const i = n.parent.children.indexOf(n); if (i >= 0) n.parent.children.splice(i, 1); n.parent = undefined; }, parentNode: (n) => n.parent ?? null, nextSibling: (n) => n.parent ? n.parent.children[n.parent.children.indexOf(n) + 1] ?? null : null,
});
function component(): Component {
  const source = readFileSync(new URL("./RewardReports.vue", import.meta.url), "utf8"), descriptor = parse(source, { filename: "RewardReports.vue" }).descriptor;
  const javascript = ts.transpileModule(compileScript(descriptor, { id: "reward-report-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./reward-report-api": RewardReportApi };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
function report(url: URL) {
  const q = url.searchParams, kind = url.pathname.includes("reward-orders") ? "reward_orders" : "rewards", group = q.get("group_by") ?? "day";
  const parts = new Intl.DateTimeFormat("en", { timeZone: "Asia/Singapore", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date(q.get("from")!));
  const day = `${parts.find((p) => p.type === "year")!.value}-${parts.find((p) => p.type === "month")!.value}-${parts.find((p) => p.type === "day")!.value}`;
  const key = group === "state" ? "granted" : group === "member" ? member : group === "order" ? order : day;
  const items = kind === "reward_orders" && group === "state"
    ? [{ key: "granted", label: "granted", totals: grantedOnly }, { key: "revocation_pending", label: "revocation_pending", totals: pendingOnly }, { key: "revoked", label: "revoked", totals: revokedOnly }]
    : [{ key, label: key, totals: kind === "rewards" ? posting : orderTotals }];
  const summary = kind === "rewards" ? posting : orderTotals;
  return { success: true, request_id: "reward-report-req-1", data: { brand_id: brand, snapshot_at: "2026-10-08T00:00:00Z", timezone: "Asia/Singapore", query: { from: q.get("from"), to: q.get("to"), group_by: group, limit: Number(q.get("limit")), offset: Number(q.get("offset")), member_id: q.get("member_id"), order_id: q.get("order_id") }, summary, items, total_groups: String(items.length) } };
}
function text(node: Node): string { return node.text + node.children.map(text).join(""); }
function find(node: Node, predicate: (n: Node) => boolean): Node | null { if (predicate(node)) return node; for (const child of node.children) { const match = find(child, predicate); if (match) return match; } return null; }
function testId(root: Node, id: string) { return find(root, (node) => node.props["data-testid"] === id); }
function click(node: Node | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
function setModel(node: Node | null, value: unknown) { expect(node).not.toBeNull(); (node!.props["onUpdate:modelValue"] as ((v: unknown) => void) | undefined)?.(value); }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function mount(initial = account) {
  const scope = ref({ account: initial, brandId: brand }), emitted: string[] = [], root = element("root");
  const RewardReportComponent = component();
  const app = renderer.createApp({ setup: () => () => h(RewardReportComponent, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, createAdminI18n()); app.mount(root); return { app, root, scope, emitted };
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("RewardReports real Vue component", () => {
  it("renders and queries both complete report modes, then exports the committed filter scope", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const fetcher = vi.fn<typeof fetch>(async (input, init) => { const url = new URL(String(input), "http://local"); calls.push({ url: url.toString(), init }); return url.pathname.endsWith(".csv") ? new Response("unavailable", { status: 503 }) : new Response(JSON.stringify(report(url)), { status: 200 }); });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount();
    expect(testId(mounted.root, "reward-report-query")?.props.disabled).toBe(false);
    expect(testId(mounted.root, "reward-report-mode-postings")).not.toBeNull();
    click(testId(mounted.root, "reward-report-query")); await flush();
    expect(new URL(calls[0]!.url).pathname).toBe("/api/v1/admin/reports/rewards");
    const postingSummary = text(testId(mounted.root, "reward-report-summary")!);
    for (const label of ["入账条目", "发放条目", "发放积分", "撤销条目", "撤销积分", "净积分变化"]) expect(postingSummary).toContain(label);
    expect(postingSummary).toContain(huge.replace(/\B(?=(\d{3})+(?!\d))/g, ","));
    expect(postingSummary).toContain("−7");
    const committedFrom = new URL(calls[0]!.url).searchParams.get("from");
    const filters = testId(mounted.root, "reward-report-filters")!, dateInput = find(filters, (n) => n.tag === "input" && n.props.type === "datetime-local");
    setModel(dateInput, "2026-10-01T00:00:00"); await flush(); click(testId(mounted.root, "reward-report-export")); await flush();
    expect(new URL(calls[1]!.url).searchParams.get("from")).toBe(committedFrom);
    expect(new URL(calls[1]!.url).pathname).toBe("/api/v1/admin/reports/rewards.csv");
    expect(new URL(calls[1]!.url).searchParams.has("offset")).toBe(false);

    click(testId(mounted.root, "reward-report-mode-orders")); await flush();
    const groupSelect = testId(mounted.root, "reward-report-group");
    expect(groupSelect?.options.map((option) => option.value)).toEqual(["day", "member", "state"]);
    const orders = text(mounted.root);
    expect(orders).toContain("按订单创建时间筛选 cohort");
    setModel(groupSelect, "state"); await flush(); click(testId(mounted.root, "reward-report-query")); await flush();
    expect(new URL(calls[2]!.url).pathname).toBe("/api/v1/admin/reports/reward-orders");
    expect(new URL(calls[2]!.url).searchParams.get("group_by")).toBe("state");
    const orderSummary = text(testId(mounted.root, "reward-report-summary")!);
    for (const label of ["订单数", "原始积分", "已发放订单", "已发放积分", "待处理订单", "待处理积分", "已撤销订单", "已撤销积分"]) expect(orderSummary).toContain(label);
    expect(orderSummary).toContain(huge.replace(/\B(?=(\d{3})+(?!\d))/g, ","));
    expect(text(mounted.root)).toContain("revocation_pending");
    expect(text(mounted.root)).toContain("撤销待处理");
    click(testId(mounted.root, "reward-report-mode-postings")); await flush();
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();
    expect(testId(mounted.root, "reward-report-group")?.options.map((option) => option.value)).toEqual(["day", "member", "order"]);
    mounted.app.unmount();
  });
  it("allows report-only view, revokes stale read/export generations on mode or scope changes, and emits on 401", async () => {
    let resolveRead!: (response: Response) => void, resolveExport!: (response: Response) => void, firstRead = true, firstExport = true;
    const readWait = new Promise<Response>((resolve) => { resolveRead = resolve; }), exportWait = new Promise<Response>((resolve) => { resolveExport = resolve; });
    const calls: { url: URL; init?: RequestInit }[] = [];
    const fetcher = vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(String(input), "http://local"); calls.push({ url, init });
      if (url.pathname.endsWith(".csv")) { if (firstExport) { firstExport = false; return exportWait; } return new Response(JSON.stringify({ error: { message: "sensitive server detail" } }), { status: 401 }); }
      if (firstRead) { firstRead = false; return readWait; }
      if (calls.filter((call) => !call.url.pathname.endsWith(".csv")).length >= 4) return new Response(JSON.stringify({ error: { message: "sensitive server detail" } }), { status: 401 });
      return new Response(JSON.stringify(report(url)), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const reportOnly = { ...account, permissions: [], permissions_by_brand: { [brand]: ["report_reward.view.brand"] } };
    const mounted = mount(reportOnly);
    expect(testId(mounted.root, "reward-report-query")?.props.disabled).toBe(false);
    click(testId(mounted.root, "reward-report-query")); await flush();
    const staleReadSignal = calls[0]!.init?.signal as AbortSignal;
    click(testId(mounted.root, "reward-report-mode-orders")); await flush();
    expect(staleReadSignal.aborted).toBe(true);
    resolveRead(new Response(JSON.stringify(report(new URL(calls[0]!.url))), { status: 200 })); await flush();
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();
    click(testId(mounted.root, "reward-report-query")); await flush();
    expect(testId(mounted.root, "reward-report-summary")).not.toBeNull();
    expect(testId(mounted.root, "reward-report-export")?.props.disabled).toBe(true);

    mounted.scope.value = { account: { ...account, permissions_by_brand: { [brand]: ["report_reward.view.brand", "report_reward.export.brand"] } }, brandId: brand }; await flush();
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();
    click(testId(mounted.root, "reward-report-query")); await flush();
    click(testId(mounted.root, "reward-report-export")); await flush();
    expect(calls.at(-1)?.url.pathname).toContain(".csv");
    const staleExportSignal = calls.at(-1)!.init?.signal as AbortSignal;
    click(testId(mounted.root, "reward-report-mode-postings")); await flush();
    expect(staleExportSignal.aborted).toBe(true);
    resolveExport(new Response("late export should be ignored", { status: 200 })); await flush();
    expect(find(mounted.root, (n) => n.props.role === "alert")).toBeNull();
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();

    click(testId(mounted.root, "reward-report-query")); await flush();
    expect(mounted.emitted).toContain("session-invalid");
    expect(text(mounted.root)).not.toContain("sensitive server detail");
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();
    mounted.app.unmount();
  });
  it("aborts an in-flight read and discards its response when the selected brand and grants change", async () => {
    const otherBrand = "66666666-6666-4666-8666-666666666666";
    let resolve!: (response: Response) => void;
    const wait = new Promise<Response>((done) => { resolve = done; }), calls: { url: URL; init?: RequestInit }[] = [];
    const fetcher = vi.fn<typeof fetch>(async (input, init) => { const url = new URL(String(input), "http://local"); calls.push({ url, init }); return wait; });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); click(testId(mounted.root, "reward-report-query")); await flush();
    const signal = calls[0]!.init?.signal as AbortSignal;
    mounted.scope.value = { account: { ...account, id: actor2, brand_ids: [otherBrand], permissions_by_brand: { [otherBrand]: ["report_reward.view.brand"] } }, brandId: otherBrand }; await flush();
    expect(signal.aborted).toBe(true);
    resolve(new Response(JSON.stringify(report(calls[0]!.url)), { status: 200 })); await flush();
    expect(testId(mounted.root, "reward-report-summary")).toBeNull();
    expect(calls).toHaveLength(1);
    mounted.app.unmount();
  });
});
