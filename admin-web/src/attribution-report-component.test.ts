import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as AttributionApi from "./attribution-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", agent = "33333333-3333-4333-8333-333333333333", huge = "922337203685477580812345678901234567890";
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_attribution.view.brand", "report_attribution.export.brand"] } };
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
  const source = readFileSync(new URL("./AttributionReportManagement.vue", import.meta.url), "utf8"), descriptor = parse(source, { filename: "AttributionReportManagement.vue" }).descriptor;
  const javascript = ts.transpileModule(compileScript(descriptor, { id: "attribution-report-component-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./attribution-report-api": AttributionApi };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
function payload(query: Record<string, unknown>, key = "2026-10-07", label = key) {
  const totals = { order_count: "1", stake_points: huge, placed_count: "0", won_count: "0", lost_count: "1", abnormal_count: "0", cancelled_count: "0", refund_points: "0", settled_stake_points: huge, unfinalized_stake_points: "0", abnormal_stake_points: "0", current_prize_points: "0", correction_open_count: "0", final_lost_stake_points: huge };
  return { success: true, request_id: "attribution-test", data: { brand_id: brand, snapshot_at: "2026-10-08T00:00:00Z", timezone: "Asia/Singapore", query, summary: totals, items: [{ key, label, totals }], total_groups: "1" } };
}
function text(node: Node): string { return node.text + node.children.map(text).join(""); }
function find(node: Node, predicate: (n: Node) => boolean): Node | null { if (predicate(node)) return node; for (const child of node.children) { const found = find(child, predicate); if (found) return found; } return null; }
function testId(root: Node, id: string) { return find(root, (n) => n.props["data-testid"] === id); }
function inputByType(root: Node, type: string) { return find(root, (n) => n.tag === "input" && n.props.type === type); }
function click(node: Node | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function mount(user = account) {
  const scope = ref({ account: user, brandId: brand }), emitted: string[] = [], root = element("root"), i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(component(), { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(root); return { app, root, scope, emitted, i18n };
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("AttributionReportManagement real Vue component", () => {
  it("waits for explicit submission, renders precise values, and ignores responses after a scope change", async () => {
    const calls: string[] = []; let resolveInitial!: (value: Response) => void, delayed = true;
    const pending = new Promise<Response>((resolve) => { resolveInitial = resolve; });
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const url = String(input); calls.push(url); if (delayed) { delayed = false; return pending; }
      const q = new URL(url, "http://local").searchParams;
      return new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), game_id: q.get("game_id"), member_id: q.get("member_id"), agent_id: q.get("agent_id"), agent_scope: q.get("agent_scope"), join_method: q.get("join_method") })), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); await flush();
    expect(fetcher).not.toHaveBeenCalled();
    click(testId(mounted.root, "attribution-query")); await flush(); expect(text(mounted.root)).toContain("查询中");
    mounted.scope.value = { account: { ...account, id: "44444444-4444-4444-8444-444444444444" }, brandId: brand }; await flush();
    const q = new URL(calls[0]!, "http://local").searchParams;
    resolveInitial(new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), game_id: null, member_id: null, agent_id: null, agent_scope: "direct", join_method: null })), { status: 200 })); await flush();
    expect(testId(mounted.root, "attribution-result")).toBeNull();
    click(testId(mounted.root, "attribution-query")); await flush();
    expect(text(mounted.root)).toContain("922,337,203,685,477,580,812,345,678,901,234,567,890");
    expect(text(mounted.root)).toContain("最终未中奖投注"); expect(testId(mounted.root, "attribution-summary")).not.toBeNull();
    mounted.i18n.setLocale("en"); await flush(); expect(text(mounted.root)).toContain("Final lost stakes");
    mounted.app.unmount();
  });
  it("does not render or query without a scoped view grant, including for a super admin", async () => {
    const fetcher = vi.fn<typeof fetch>(); vi.stubGlobal("fetch", fetcher);
    const noView = { ...account, super_admin: true, permissions_by_brand: { [brand]: ["report_attribution.export.brand"] } };
    const mounted = mount(noView); await flush();
    expect(text(mounted.root)).not.toContain("归因报表"); expect(fetcher).not.toHaveBeenCalled(); mounted.app.unmount();
  });
  it("exports the committed filters without list pagination", async () => {
    const calls: string[] = [];
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const url = String(input); calls.push(url);
      if (url.includes("/export?")) return new Response(JSON.stringify({ error: { message: "Export unavailable" } }), { status: 503, headers: { "content-type": "application/json" } });
      const q = new URL(url, "http://local").searchParams;
      return new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), game_id: null, member_id: null, agent_id: null, agent_scope: "direct", join_method: null })), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); click(testId(mounted.root, "attribution-query")); await flush(); click(testId(mounted.root, "attribution-export")); await flush();
    const exported = new URL(calls.at(-1)!, "http://local");
    expect(exported.pathname).toBe("/api/v1/admin/reports/attribution/export"); expect(exported.searchParams.has("limit")).toBe(false); expect(exported.searchParams.has("offset")).toBe(false);
    expect(text(mounted.root)).toContain("Export unavailable"); mounted.app.unmount();
  });
  it("rejects an impossible calendar date before sending a request", async () => {
    const fetcher = vi.fn<typeof fetch>(); vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); const start = inputByType(mounted.root, "datetime-local"); expect(start).not.toBeNull();
    (start!.props["onUpdate:modelValue"] as ((value: string) => void))("2026-02-30T12:00:00"); await flush(); click(testId(mounted.root, "attribution-query")); await flush();
    expect(fetcher).not.toHaveBeenCalled(); expect(find(mounted.root, (n) => n.props.role === "alert")).not.toBeNull(); mounted.app.unmount();
  });
  it("labels the agent-none group and offers only current join methods", async () => {
    let key = "none", label = "none";
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const q = new URL(String(input), "http://local").searchParams;
      return new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), game_id: q.get("game_id"), member_id: q.get("member_id"), agent_id: q.get("agent_id"), agent_scope: q.get("agent_scope"), join_method: q.get("join_method") }, key, label)), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); mounted.i18n.setLocale("en"); await flush();
    const groupSelect = testId(mounted.root, "attribution-group"); expect(groupSelect).not.toBeNull();
    expect(groupSelect!.props["aria-label"]).toBe("Group");
    expect(testId(mounted.root, "attribution-agent-scope")!.props["aria-label"]).toBe("Agent scope");
    expect(testId(mounted.root, "attribution-join-method")!.props["aria-label"]).toBe("Join method");
    const selectGroup = async (group: string) => { (groupSelect!.props["onUpdate:modelValue"] as (value: string) => void)(group); await flush(); click(testId(mounted.root, "attribution-query")); await flush(); };
    await selectGroup("agent"); expect(text(mounted.root)).toContain("No agent"); expect(text(mounted.root)).not.toContain("No identity");
    const joinMethodSelect = testId(mounted.root, "attribution-join-method");
    expect(joinMethodSelect!.options.map((option) => option.value)).not.toContain("legacy");
    key = actor; label = actor;
    await selectGroup("member"); expect(text(mounted.root)).toContain(actor); expect(text(mounted.root)).not.toContain("No identity");
    mounted.app.unmount();
  });
  it("shows an honest zero range for an empty result and explains the two timezones", async () => {
    const zero = { order_count: "0", stake_points: "0", placed_count: "0", won_count: "0", lost_count: "0", abnormal_count: "0", cancelled_count: "0", refund_points: "0", settled_stake_points: "0", unfinalized_stake_points: "0", abnormal_stake_points: "0", current_prize_points: "0", correction_open_count: "0", final_lost_stake_points: "0" };
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const q = new URL(String(input), "http://local").searchParams;
      const body = payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), game_id: null, member_id: null, agent_id: null, agent_scope: "direct", join_method: null });
      body.data.summary = zero; body.data.items = []; body.data.total_groups = "0";
      return new Response(JSON.stringify(body), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); await flush();
    expect(text(mounted.root)).toContain("时间按设备本地时区输入并转换为 UTC；按天分组使用品牌时区。");
    click(testId(mounted.root, "attribution-query")); await flush();
    expect(text(mounted.root)).toContain("0–0"); expect(text(mounted.root)).toContain("0 组");
    mounted.i18n.setLocale("en"); await flush(); expect(text(mounted.root)).toContain("Enter dates in the device timezone; requests use UTC, and day groups use the brand timezone.");
    mounted.app.unmount();
  });
});
