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
import * as CommissionReportApi from "./commission-report-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "22222222-2222-4222-8222-222222222222", huge = "922337203685477580812345678901234567890";
const account: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_commission.view.brand", "report_commission.export.brand"] } };
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
  const source = readFileSync(new URL("./CommissionReport.vue", import.meta.url), "utf8"), descriptor = parse(source, { filename: "CommissionReport.vue" }).descriptor;
  const javascript = ts.transpileModule(compileScript(descriptor, { id: "commission-report-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commission-report-api": CommissionReportApi };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body)(modules) as Component;
}
const payload = (query: Record<string, unknown>) => ({ success: true, request_id: "read-1", data: { brand_id: brand, snapshot_at: "2026-10-08T00:00:00Z", timezone: "Asia/Singapore", query, summary: { entry_count: "1", paid_entry_count: "1", paid_points: huge, adjustment_entry_count: "0", adjustment_credit_points: "0", adjustment_debit_points: "0", correction_entry_count: "0", correction_credit_points: "0", correction_debit_points: "0", net_points: huge }, items: [{ key: query.group_by === "day" ? "2026-10-07" : actor, label: query.group_by === "day" ? "2026-10-07" : actor, totals: { entry_count: "1", paid_entry_count: "1", paid_points: huge, adjustment_entry_count: "0", adjustment_credit_points: "0", adjustment_debit_points: "0", correction_entry_count: "0", correction_credit_points: "0", correction_debit_points: "0", net_points: huge } }], total_groups: "1" } });
function text(node: Node): string { return node.text + node.children.map(text).join(""); }
function find(node: Node, pred: (n: Node) => boolean): Node | null { if (pred(node)) return node; for (const child of node.children) { const match = find(child, pred); if (match) return match; } return null; }
function testId(root: Node, id: string) { return find(root, (node) => node.props["data-testid"] === id); }
function button(root: Node, caption: string) { return find(root, (n) => n.tag === "button" && text(n).includes(caption)); }
function click(node: Node | null) { expect(node).not.toBeNull(); (node!.props.onClick as (() => void) | undefined)?.(); }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
function mount() {
  const scope = ref({ account, brandId: brand }), emitted: string[] = [], root = element("root"), i18n = createAdminI18n();
  const app = renderer.createApp({ setup: () => () => h(component(), { ...scope.value, onSessionInvalid: () => emitted.push("session-invalid") }) });
  app.provide(adminI18nKey, i18n); app.mount(root); return { app, root, scope, emitted, i18n };
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });
describe("CommissionReport real Vue component", () => {
  it("renders exact large values, exports committed filters, and discards late responses after scope change", async () => {
    const calls: string[] = []; let resolveFirst!: (value: Response) => void, first = true, exportStatus = 503;
    const slow = new Promise<Response>((resolve) => { resolveFirst = resolve; });
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const url = String(input); calls.push(url);
      if (url.includes(".csv?")) return new Response("", { status: exportStatus, headers: { "content-type": "application/json" } });
      if (first) { first = false; return slow; }
      const q = new URL(url, "http://local").searchParams;
      return new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), agent_id: q.get("agent_id"), member_id: q.get("member_id"), cycle_id: q.get("cycle_id") })), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); click(testId(mounted.root, "commission-report-query")); await flush();
    expect(text(mounted.root)).toContain("读取中");
    mounted.scope.value = { account: { ...account, id: "33333333-3333-4333-8333-333333333333" }, brandId: brand }; await flush();
    resolveFirst(new Response(JSON.stringify(payload({ from: "2026-10-01T00:00:00Z", to: "2026-10-08T00:00:00Z", group_by: "day", limit: 20, offset: 0, agent_id: null, member_id: null, cycle_id: null })), { status: 200 })); await flush();
    expect(text(mounted.root)).not.toContain(huge);
    click(testId(mounted.root, "commission-report-query")); await flush();
    expect(text(mounted.root)).toContain("922,337,203,685,477,580,812,345,678,901,234,567,890");
    expect(text(mounted.root)).toContain("更正笔数"); expect(text(mounted.root)).toContain("更正补发积分"); expect(text(mounted.root)).toContain("更正追回积分");
    mounted.i18n.setLocale("en"); await flush();
    expect(text(mounted.root)).toContain("Correction entries"); expect(text(mounted.root)).toContain("Correction credits"); expect(text(mounted.root)).toContain("Correction debits");
    expect(testId(mounted.root, "commission-report-summary")).not.toBeNull();
    expect(testId(mounted.root, "commission-report-group")).not.toBeNull();
    const datetime = find(mounted.root, (n) => n.tag === "input" && n.props.type === "datetime-local"); expect(datetime).not.toBeNull();
    (datetime!.props["onUpdate:modelValue"] as ((value: string) => void))("2020-01-01T00:00:00");
    click(testId(mounted.root, "commission-report-export")); await flush();
    const readUrl = new URL(calls.filter((url) => !url.includes(".csv?" )).at(-1)!, "http://local"), exportUrl = new URL(calls.at(-1)!, "http://local");
    expect(exportUrl.searchParams.get("from")).toBe(readUrl.searchParams.get("from"));
    expect(text(mounted.root)).toContain("Request failed (503)");
    exportStatus = 401; click(testId(mounted.root, "commission-report-export")); await flush();
    expect(mounted.emitted).toContain("session-invalid");
    expect(testId(mounted.root, "commission-report-summary")).toBeNull();
    mounted.app.unmount();
  });
  it("rejects a nonexistent calendar date rather than allowing Date rollover", async () => {
    const fetcher = vi.fn<typeof fetch>(async (input) => {
      const q = new URL(String(input), "http://local").searchParams;
      return new Response(JSON.stringify(payload({ from: q.get("from"), to: q.get("to"), group_by: q.get("group_by"), limit: Number(q.get("limit")), offset: Number(q.get("offset")), agent_id: q.get("agent_id"), member_id: q.get("member_id"), cycle_id: q.get("cycle_id") })), { status: 200 });
    });
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const mounted = mount(); click(testId(mounted.root, "commission-report-query")); await flush();
    const fromInput = find(mounted.root, (n) => n.tag === "input" && n.props.type === "datetime-local"); expect(fromInput).not.toBeNull();
    (fromInput!.props["onUpdate:modelValue"] as ((value: string) => void))("2026-02-30T12:00:00");
    click(testId(mounted.root, "commission-report-query")); await flush();
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(find(mounted.root, (n) => n.props.role === "alert")).not.toBeNull();
    mounted.app.unmount();
  });
});
