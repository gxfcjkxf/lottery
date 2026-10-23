import { h as temporalTestH } from "vue";
import * as DisplayShared from "@lottery/shared";
DisplayShared.setPlatformDisplaySettings({ timezone: "Asia/Singapore", date_format: "YYYY-MM-DD", time_format: "HH:mm:ss" });
import { afterEach, describe, expect, it, vi } from "vitest";
import { createRenderer, h, nextTick, type Component } from "vue";
import * as VueRuntime from "vue";
import { readFileSync } from "node:fs";
import { compileScript, parse } from "vue/compiler-sfc";
import ts from "typescript";
import * as ReportsApi from "./reports-api";
import * as WithdrawalApi from "./withdrawal-report-api";
import * as ReportExportApi from "./report-export-api";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as AnalysisApi from "./commission-analysis-api";
import * as RuleVersionsApi from "./rule-versions-api";
import * as Shared from "@lottery/shared";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";

function component(filename = "ReportsManagement.vue"): Component {
  const source = readFileSync(new URL(`./${filename}`, import.meta.url), "utf8");
  const descriptor = parse(source, { filename }).descriptor;
  const js = ts.transpileModule(compileScript(descriptor, { id: "reports-locale-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const empty = { render: () => null };
  const memberLookup = { lookupAdminMember: async () => { throw new Error("Member lookup failed"); } };
  const modules: Record<string, unknown> = { "@lottery/shared/FilterAssociationSelect.vue": { props: ["modelValue", "label"], setup: (p: any, { emit }: any) => () => temporalTestH("select", { value: p.modelValue, "aria-label": p.label, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) },  "@lottery/shared/PlatformTemporalInput.vue": { props: ["modelValue", "type", "required", "disabled"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { type: p.type, value: p.modelValue, required: p.required, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, vue: VueRuntime, "@lottery/shared": { ...DisplayShared, ...Shared }, "./admin-api": AdminApi, "./i18n": AdminI18n, "./reports-api": ReportsApi, "./withdrawal-report-api": WithdrawalApi, "./report-export-api": ReportExportApi, "./member-lookup": memberLookup, "./rule-versions-api": RuleVersionsApi, "./WithdrawalReport.vue": empty, "./CommissionReport.vue": empty, "./CommissionAnalysisReport.vue": realCommissionAnalysisChild(), "./RewardReports.vue": empty, "./AttributionReportManagement.vue": empty };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`)
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, name: string, specifier: string) => `const ${name}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body.replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
}

function realCommissionAnalysisChild(): Component {
  const filename = "CommissionAnalysisReport.vue", source = readFileSync(new URL(`./${filename}`, import.meta.url), "utf8"), descriptor = parse(source, { filename }).descriptor;
  const js = ts.transpileModule(compileScript(descriptor, { id: "reports-real-commission-analysis", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { "@lottery/shared/FilterAssociationSelect.vue": { props: ["modelValue", "label"], setup: (p: any, { emit }: any) => () => temporalTestH("select", { value: p.modelValue, "aria-label": p.label, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "@lottery/shared/PlatformTemporalInput.vue": { props: ["modelValue", "type", "required", "disabled"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { type: p.type, value: p.modelValue, required: p.required, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "@lottery/shared": DisplayShared,  vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commission-analysis-api": AnalysisApi, "./member-lookup": { lookupAdminMember: async () => { throw new Error("Member lookup failed"); } } };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body.replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
}

type Node = { tag: string; text: string; props: Record<string, unknown>; children: Node[]; parent?: Node; value?: string; style: Record<string, string>; addEventListener: () => void; removeEventListener: () => void; getRootNode: () => Node; options: Node[] };
function element(tag: string): Node {
  return { tag, text: "", props: {}, children: [], value: "", style: {}, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter(n => n.tag === "option"); } };
}
const renderer = createRenderer<Node, Node>({
  createElement: element, createText: text => ({ ...element("#text"), text }), createComment: () => element("#comment"),
  setText: (n, text) => { n.text = text; }, setElementText: (n, text) => { n.text = text; n.children = []; },
  patchProp: (n, key, _old, value) => { n.props[key] = value; if (key === "value") n.value = String(value ?? ""); },
  insert(n, parent, anchor) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); n.parent = parent; const index = anchor ? parent.children.indexOf(anchor) : -1; if (index < 0) parent.children.push(n); else parent.children.splice(index, 0, n); },
  remove(n) { if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1); }, parentNode: n => n.parent ?? null, nextSibling: n => n.parent?.children[n.parent.children.indexOf(n) + 1] ?? null,
});
function nodes(root: Node): Node[] { return [root, ...root.children.flatMap(nodes)]; }
function text(root: Node): string { return root.text + root.children.map(text).join(""); }
function button(root: Node, label: string) { const n = nodes(root).find(n => n.tag === "button" && text(n) === label); expect(n).toBeDefined(); return n!; }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
const brand = "11111111-1111-4111-8111-111111111111";
const account: AdminAccount = { id: "22222222-2222-4222-8222-222222222222", super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_betting.view.brand", "report_ledger.view.brand"] } };
const mounted: { unmount: () => void }[] = [];
function mount() {
  const root = element("root"), i18n = createAdminI18n();
  const target = component();
  const app = renderer.createApp({ setup: () => () => h(target, { account, brandId: brand }) });
  app.provide(adminI18nKey, i18n); app.mount(root); mounted.push(app); return { root, i18n };
}
afterEach(() => { mounted.splice(0).forEach(app => app.unmount()); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
describe("report localization", () => {
  it("switches actual controls without altering filter values or issuing requests", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response("", { status: 503 }));
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const { root, i18n } = mount(); await flush();
    expect(text(root)).toContain("运营报表");
    const inputs = nodes(root).filter(n => n.tag === "input"), values = inputs.map(n => n.value);
    const selects = nodes(root).filter(n => n.tag === "select"), options = selects.map(n => n.options.map(o => o.props.value));
    i18n.setLocale("en"); await flush();
    expect(text(root)).toContain("Operational reports"); expect(text(root)).not.toMatch(/\p{Script=Han}/u);
    expect(inputs.map(n => n.value)).toEqual(values); expect(selects.map(n => n.options.map(o => o.props.value))).toEqual(options);
    expect(fetcher).not.toHaveBeenCalled();
    const member = nodes(root).find(n => n.tag === "select" && n.props["aria-label"] === "Member account")!;
    (member.props["onUpdate:modelValue"] as (v: string) => void)("44444444-4444-4444-8444-444444444444");
    (button(root, "Query reports").props.onClick as () => void)(); await flush();
    expect(fetcher).toHaveBeenCalled(); expect(new URL(String(fetcher.mock.calls[0]![0]), "http://local").searchParams.get("member_id")).toBe("44444444-4444-4444-8444-444444444444");
    expect(text(root)).not.toContain("Member lookup failed");
    i18n.setLocale("zh-CN"); await flush(); expect(text(root)).toContain("暂时无法读取最新数据，请稍后刷新。");
    expect(text(root)).not.toContain("The operation could not be completed.");
    expect(member.value).toBe("44444444-4444-4444-8444-444444444444");
  });
});

describe("populated reports", () => {
  it("renders betting, ledger and balance summaries in both languages", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const betting = { order_count:"1",stake_points:"2",placed_count:"0",won_count:"1",lost_count:"0",abnormal_count:"0",cancelled_count:"0",refund_points:"0",settled_stake_points:"2",unfinalized_stake_points:"0",abnormal_stake_points:"0",current_prize_points:"4",correction_open_count:"0" };
    const ledger = { entry_count:"1",net_points:"4",recharge_points:"0",prize_credit_points:"4",prize_reversal_points:"0",refund_points:"0" };
    const base = { brand_id:brand,snapshot_at:"2026-10-20T00:00:00Z",timezone:"Asia/Singapore",query:{from:"2026-10-19T00:00:00Z",to:"2026-10-20T00:00:00Z",group_by:"day",limit:20,offset:0,game_id:null,member_id:null},total_groups:"1" };
    vi.spyOn(ReportsApi,"createReportsApi").mockReturnValue({ betting:vi.fn().mockResolvedValue({...base,summary:betting,items:[{key:"2026-10-19",label:"2026-10-19",totals:betting}]}), ledger:vi.fn().mockResolvedValue({...base,summary:ledger,items:[{key:"2026-10-19",label:"2026-10-19",totals:ledger}],balances:{account_count:"1",available_points:"4",frozen_points:"0",withdrawal_points:"0",total_points:"4"}}) });
    const {root,i18n}=mount(); await flush();
    (button(root,"查询报表").props.onClick as ()=>void)(); await flush();
    expect(text(root)).toContain("注单数");
    i18n.setLocale("en"); await flush(); expect(text(root)).toContain("Bet orders");
    (button(root,"Ledger report").props.onClick as ()=>void)(); await flush();
    (button(root,"Query reports").props.onClick as ()=>void)(); await flush();
    expect(text(root)).toContain("Ledger entries"); expect(text(root)).toContain("Wallet accounts");
    i18n.setLocale("zh-CN"); await flush(); expect(text(root)).toContain("账本笔数"); expect(text(root)).toContain("钱包账号数");
  });
  it("the withdrawal query button actually invokes its report API", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const report=vi.fn().mockRejectedValue(new AdminApi.AdminApiError("read unavailable",503));
    vi.spyOn(WithdrawalApi,"createWithdrawalReportApi").mockReturnValue({report,exportCsv:vi.fn()});
    const root=element("root"),i18n=createAdminI18n(),target=component("WithdrawalReport.vue");
    const reader={...account,permissions_by_brand:{[brand]:["report_withdrawal.view.brand"]}};
    const app=renderer.createApp({setup:()=>()=>h(target,{account:reader,brandId:brand})});app.provide(adminI18nKey,i18n);app.mount(root);mounted.push(app);
    (button(root,"查询").props.onClick as ()=>void)(); await flush();
    expect(report).toHaveBeenCalledTimes(1);expect(text(root)).toContain("暂时无法读取最新数据");
  });
});
