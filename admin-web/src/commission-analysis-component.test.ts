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
import * as AnalysisApi from "./commission-analysis-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", key = "abcdefab-cdef-4abc-8def-abcdefabcdef";
const huge = "922337203685477580812345678901234567890";
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.view.brand", "report_commission.export.brand"] } };
function component(): Component {
  const source = readFileSync(new URL("./CommissionAnalysisReport.vue", import.meta.url), "utf8"), descriptor = parse(source, { filename: "CommissionAnalysisReport.vue" }).descriptor;
  const js = ts.transpileModule(compileScript(descriptor, { id: "commission-analysis-component-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commission-analysis-api": AnalysisApi };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
type Node = { tag: string; props: Record<string, unknown>; children: Node[]; text: string; parent?: Node; value?: string; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => Node; options: Node[] };
function element(tag: string): Node { return { tag, props: {}, children: [], text: "", value: "", addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((c) => c.tag === "option"); } }; }
const renderer = createRenderer<Node, Node>({
  createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }),
  setText: (n, text) => { n.text = text; }, setElementText: (n, text) => { n.text = text; n.children = []; },
  patchProp: (n, key, _old, value) => { n.props[key] = value; if (key === "value") n.value = String(value ?? ""); },
  insert: (n, p, a) => { if (n.parent) { const i = n.parent.children.indexOf(n); if (i >= 0) n.parent.children.splice(i, 1); } n.parent = p; const i = a ? p.children.indexOf(a) : -1; if (i < 0) p.children.push(n); else p.children.splice(i, 0, n); },
  remove: (n) => { if (n.parent) { const i = n.parent.children.indexOf(n); if (i >= 0) n.parent.children.splice(i, 1); n.parent = undefined; } }, parentNode: (n) => n.parent ?? null, nextSibling: (n) => n.parent ? n.parent.children[n.parent.children.indexOf(n) + 1] ?? null : null,
});
function text(node: Node): string { return node.text + node.children.map(text).join(""); }
function find(node: Node, predicate: (n: Node) => boolean): Node | null { if (predicate(node)) return node; for (const child of node.children) { const value = find(child, predicate); if (value) return value; } return null; }
function testId(root: Node, id: string) { return find(root, (n) => n.props["data-testid"] === id); }
function click(node: Node | null) { expect(node).not.toBeNull(); if (!node!.props.disabled) (node!.props.onClick as (() => void) | undefined)?.(); }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function response(q: URLSearchParams) {
  const fullQuery = { from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), agent_id: q.get("agent_id"), member_id: q.get("member_id"), cycle_id: q.get("cycle_id") };
  const totals = { observed_calculated_points: huge, calculated_points: huge, paid_entry_count: "1", paid_points: huge, adjustment_entry_count: "0", adjustment_credit_points: "0", adjustment_debit_points: "0", correction_entry_count: "0", correction_credit_points: "0", correction_debit_points: "0", posting_entry_count: "1", actual_net_points: huge, manual_adjustment_net_points: "0", effective_target_points: huge, calculation_minus_actual_points: "0", effective_minus_actual_points: "0", calculation_complete: true, effective_target_complete: true };
  return new Response(JSON.stringify({ success: true, data: { brand_id: brand, snapshot_at: "2026-10-08T00:00:00Z", timezone: "Asia/Singapore", query: fullQuery, coverage: { selected_cycle_count: "1", ready_cycle_count: "1", unready_cycle_count: "0", legacy_policy_blocked_cycle_count: "0" }, summary: totals, items: [{ key, label: key, totals }], total_groups: "1" } }), { status: 200 });
}
function mount() {
  const root = element("root"), scope = ref({ account, brandId: brand }), emitted: string[] = [], i18n = createAdminI18n(), Child = component();
  const app = renderer.createApp({ setup: () => () => h(Child, { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(root); return { app, root, scope, emitted, i18n };
}
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("CommissionAnalysisReport real compiled Vue child", () => {
  it("is explicit-query only, renders exact large integers and clears results when a draft filter changes", async () => {
    const fetcher = vi.fn<typeof fetch>(async (input) => response(new URL(String(input), "http://local").searchParams));
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); await flush();
    expect(fetcher).not.toHaveBeenCalled();
    expect(text(mounted.root)).toContain("按保存周期的 window_to 选择完整周期");
    click(testId(mounted.root, "commission-analysis-query")); await flush();
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(text(mounted.root)).toContain("922,337,203,685,477,580,812,345,678,901,234,567,890");
    expect(text(mounted.root)).toContain("完整筛选汇总");
    expect(testId(mounted.root, "commission-analysis-summary")).not.toBeNull();
    const input = find(mounted.root, (n) => n.tag === "input"); expect(input).not.toBeNull();
    (input!.props["onUpdate:modelValue"] as (value: string) => void)("2026-01-03T00:00:00"); await flush();
    expect(testId(mounted.root, "commission-analysis-summary")).toBeNull();
    expect(testId(mounted.root, "commission-analysis-export")).toBeNull();
    mounted.app.unmount();
  });
  it("ignores a delayed 401 after account scope changes and emits only for a current 401", async () => {
    let finish!: (value: Response) => void, first = true;
    const delayed = new Promise<Response>((resolve) => { finish = resolve; });
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      if (first) { first = false; return delayed; }
      return new Response(JSON.stringify({ error: { code: "SESSION_EXPIRED", message: "expired" } }), { status: 401 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); click(testId(mounted.root, "commission-analysis-query")); await flush();
    mounted.scope.value = { account: { ...account, id: "33333333-3333-4333-8333-333333333333" }, brandId: brand }; await flush();
    finish(new Response(JSON.stringify({ error: { code: "SESSION_EXPIRED", message: "expired" } }), { status: 401 })); await flush();
    expect(mounted.emitted).toEqual([]);
    click(testId(mounted.root, "commission-analysis-query")); await flush();
    expect(mounted.emitted).toEqual(["session-invalid"]);
    mounted.app.unmount();
  });
  it("localizes controls and calculated report labels", async () => {
    const fetcher = vi.fn<typeof fetch>(); vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); mounted.i18n.setLocale("en"); await flush();
    expect(text(mounted.root)).toContain("Commission cycle calculation and actual posting analysis");
    fetcher.mockImplementation(async (input) => response(new URL(String(input), "http://local").searchParams));
    click(testId(mounted.root, "commission-analysis-query")); await flush();
    expect(text(mounted.root)).toContain("Calculation minus actual");
    expect(fetcher).toHaveBeenCalledTimes(1); mounted.app.unmount();
  });
});
