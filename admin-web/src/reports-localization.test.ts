import {ManagementListTestComponent,MemberActionsMenuTestComponent,clickHostButton} from "./management-list-test-helper"
import {TablePaginationTestComponent} from "./table-pagination-test-helper"
import { SelectControlTestComponent } from './select-control-test-helper'
import { operationToastText } from "./operation-toast-test-helper";
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
import * as AttributionApi from "./attribution-report-api";
import * as BrandPermissions from "./brand-permissions";
import * as WithdrawalApi from "./withdrawal-report-api";
import * as ReportExportApi from "./report-export-api";
import * as AdminApi from "./admin-api";
import * as AdminI18n from "./i18n";
import * as Shared from "@lottery/shared";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";

function component(filename = "ReportsManagement.vue"): Component {
  const source = readFileSync(new URL(`./${filename}`, import.meta.url), "utf8");
  const descriptor = parse(source, { filename }).descriptor;
  const js = ts.transpileModule(compileScript(descriptor, { id: "reports-locale-test", inlineTemplate: true }).content, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const empty = { render: () => null };
  const memberLookup = { lookupAdminMember: async () => { throw new Error("Member lookup failed"); } };
  const modules: Record<string, unknown> = {"@lottery/shared/ManagementList.vue":ManagementListTestComponent,"@lottery/shared/MemberActionsMenu.vue":MemberActionsMenuTestComponent, "@lottery/shared/TablePagination.vue": TablePaginationTestComponent,  "@lottery/shared/SelectControl.vue": SelectControlTestComponent, "./SelectControl.vue": SelectControlTestComponent, "@lottery/shared/FilterAssociationSearch.vue": { props: ["modelValue", "label"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { value: p.modelValue, "aria-label": p.label, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, "@lottery/shared/FilterAssociationSelect.vue": { props: ["modelValue", "label"], setup: (p: any, { emit }: any) => () => temporalTestH("select-control", { value: p.modelValue, "aria-label": p.label, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) },  "@lottery/shared/PlatformTemporalInput.vue": { props: ["modelValue", "type", "required", "disabled"], setup: (p: any, { emit }: any) => () => temporalTestH("input", { type: p.type, value: p.modelValue, required: p.required, disabled: p.disabled, "onUpdate:modelValue": (v: string) => emit("update:modelValue", v) }) }, vue: VueRuntime, "@lottery/shared": { ...DisplayShared, ...Shared, vInlineValidation: {} }, "./admin-api": AdminApi, "./i18n": AdminI18n, "./reports-api": ReportsApi, "./attribution-report-api": AttributionApi, "./brand-permissions": BrandPermissions, "./withdrawal-report-api": WithdrawalApi, "./report-export-api": ReportExportApi, "./member-lookup": memberLookup, "./WithdrawalReport.vue": empty, "./CommissionReport.vue": empty, "./RewardReports.vue": empty };
  const body = js.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`)
    .replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, name: string, specifier: string) => `const ${name}=__modules[${JSON.stringify(specifier)}];`).replace(/export\s+default\s+/, "return ");
  return new Function("__modules", body.replace(/^import\s+["'][^"']+\.css["'];?\s*$/gm, "").replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, binding: string, specifier: string) => `const ${binding}=__modules[${JSON.stringify(specifier)}];`))(modules) as Component;
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
function button(root: Node, label: string) { const n = nodes(root).find(n => n.tag === "button" && (text(n) === label || n.props["aria-label"] === label)); expect(n).toBeDefined(); return n!; }
async function flush() { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); }
const brand = "11111111-1111-4111-8111-111111111111";
const account: AdminAccount = { id: "22222222-2222-4222-8222-222222222222", super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: ["report_betting.view.brand", "report_ledger.view.brand"] } };
const mounted: { unmount: () => void }[] = [];
function mount(reader = account) {
  const root = element("root"), i18n = createAdminI18n();
  const target = component();
  const app = renderer.createApp({ setup: () => () => h(target, { account: reader, brandId: brand }) });
  app.provide(adminI18nKey, i18n); app.mount(root); mounted.push(app); return { root, i18n };
}
afterEach(() => { mounted.splice(0).forEach(app => app.unmount()); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
describe("report localization", () => {
  it("starts with posting activity for a commission-only reader", async () => {
    const { root } = mount({ ...account, permissions_by_brand: { [brand]: ["commission.view.brand", "report_commission.view.brand"] } });
    await flush();
    const selected = nodes(root).filter(n => n.props.role === "tab" && n.props["aria-selected"]);
    expect(selected).toHaveLength(1);
    expect(text(selected[0]!)).toBe("入账统计");
  });
  it("switches actual controls without altering filter values or issuing requests", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response("", { status: 503 }));
    vi.stubGlobal("fetch", fetcher); vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const { root, i18n } = mount(); await flush();
    expect(text(root) + operationToastText()).toContain("业务报表");
    const inputs = nodes(root).filter(n => n.tag === "input"), values = inputs.map(n => n.value);
    const selects = nodes(root).filter(n => n.tag === "select-control"), options = selects.map(n => n.options.map(o => o.props.value));
    const initialReads = fetcher.mock.calls.length;
    expect(initialReads).toBe(2);
    i18n.setLocale("en"); await flush();
    expect(text(root) + operationToastText()).toContain("Business reports"); expect(text(root) + operationToastText()).not.toMatch(/\p{Script=Han}/u);
    expect(inputs.map(n => n.value)).toEqual(values); expect(selects.map(n => n.options.map(o => o.props.value))).toEqual(options);
    expect(fetcher).toHaveBeenCalledTimes(initialReads);
    fetcher.mockClear();
    const member = nodes(root).find(n => n.tag === "input" && n.props["aria-label"] === "Member account")!;
    (member.props["onUpdate:modelValue"] as (v: string) => void)("44444444-4444-4444-8444-444444444444");
    clickHostButton(button(root, "Query")); await flush();
    expect(fetcher).toHaveBeenCalled(); expect(new URL(String(fetcher.mock.calls[0]![0]), "http://local").searchParams.get("member_id")).toBe("44444444-4444-4444-8444-444444444444");
    expect(text(root) + operationToastText()).not.toContain("Member lookup failed");
    i18n.setLocale("zh-CN"); await flush(); expect(text(root) + operationToastText()).toContain("暂时无法读取最新数据，请稍后刷新。");
    expect(text(root) + operationToastText()).not.toContain("The operation could not be completed.");
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
    clickHostButton(button(root,"查询")); await flush();
    expect(text(root) + operationToastText()).toContain("注单数");
    i18n.setLocale("en"); await flush(); expect(text(root) + operationToastText()).toContain("Bet orders");
    clickHostButton(button(root,"Ledger report")); await flush();
    clickHostButton(button(root,"Query")); await flush();
    expect(text(root) + operationToastText()).toContain("Ledger entries"); expect(text(root) + operationToastText()).toContain("Wallet accounts");
    i18n.setLocale("zh-CN"); await flush(); expect(text(root) + operationToastText()).toContain("账本笔数"); expect(text(root) + operationToastText()).toContain("钱包账号数");
  });
  it("the withdrawal query button actually invokes its report API", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const report=vi.fn().mockRejectedValue(new AdminApi.AdminApiError("read unavailable",503));
    vi.spyOn(WithdrawalApi,"createWithdrawalReportApi").mockReturnValue({report,exportCsv:vi.fn()});
    const root=element("root"),i18n=createAdminI18n(),target=component("WithdrawalReport.vue");
    const reader={...account,permissions_by_brand:{[brand]:["report_withdrawal.view.brand"]}};
    const app=renderer.createApp({setup:()=>()=>h(target,{account:reader,brandId:brand})});app.provide(adminI18nKey,i18n);app.mount(root);mounted.push(app);
    clickHostButton(button(root,"查询")); await flush();
    expect(report).toHaveBeenCalledTimes(2);expect(text(root) + operationToastText()).toContain("暂时无法读取最新数据");
  });
});


describe("merged betting filters", () => {
  it("keeps agent/source filters on pagination without exposing export", async () => {
    vi.stubGlobal("Document", class {}); vi.stubGlobal("ShadowRoot", class {});
    const fetcher = vi.fn<typeof fetch>(async () => new Response("", { status: 503 }));
    vi.stubGlobal("fetch", fetcher);
    const agent = "44444444-4444-4444-8444-444444444444";
    const totals = { order_count:"1",stake_points:"1",placed_count:"1",won_count:"0",lost_count:"0",abnormal_count:"0",cancelled_count:"0",refund_points:"0",settled_stake_points:"0",unfinalized_stake_points:"1",abnormal_stake_points:"0",current_prize_points:"0",correction_open_count:"0",final_lost_stake_points:"0" };
    const report = vi.fn(async (_brand: string, q: AttributionApi.AttributionReportQuery): Promise<AttributionApi.AttributionReport> => ({
      brand_id: brand, snapshot_at: "2026-10-27T00:00:00Z", timezone: "Asia/Singapore",
      query: { from:q.from,to:q.to,group_by:q.group_by,limit:q.limit??20,offset:q.offset??0,game_id:q.game_id??null,member_id:q.member_id??null,agent_id:q.agent_id??null,agent_scope:q.agent_scope??"direct",join_method:q.join_method??null },
      summary: { ...totals, order_count:"21",stake_points:"21",placed_count:"21",unfinalized_stake_points:"21" },
      items: Array.from({ length: q.offset ? 1 : 20 }, (_, i) => ({ key: `00000000-0000-4000-8000-${String((q.offset??0)+i+1).padStart(12,"0")}`, label: "member", totals })), total_groups:"21",
    }));
    const exportCsv = vi.fn(async () => { throw new AdminApi.AdminApiError("Unavailable",503); });
    vi.spyOn(AttributionApi, "createAttributionReportApi").mockReturnValue({report,exportCsv});
    const {root}=mount({...account,permissions_by_brand:{[brand]:[...account.permissions_by_brand![brand]!,"report_attribution.view.brand","report_attribution.export.brand"]}});
    await flush();
    const input=nodes(root).find(n=>n.tag==="input"&&n.props["aria-label"]==="代理账号")!;
    (input.props["onUpdate:modelValue"] as (value:string)=>void)(agent);
    const grouping=nodes(root).find(n=>n.tag==="select-control"&&n.options.some(o=>o.props.value==="join_method"))!;
    (grouping.props["onUpdate:modelValue"] as (value:string)=>void)("member");
    clickHostButton(button(root,"查询"));await flush();
    expect(report).toHaveBeenLastCalledWith(brand,expect.objectContaining({agent_id:agent,group_by:"member",offset:0}));
    (input.props["onUpdate:modelValue"] as (value:string)=>void)("55555555-5555-4555-8555-555555555555");
    const results=nodes(root).find(n=>String(n.props.class).includes("management-list")&&n.children.some(child=>nodes(child).some(node=>node.props.id==="betting-results")))!;
    clickHostButton(button(results,"下一页"));await flush();
    expect(report).toHaveBeenLastCalledWith(brand,expect.objectContaining({agent_id:agent,group_by:"member",offset:20}));
    expect(nodes(root).some(n=>n.tag==="button"&&text(n)==="导出完整 CSV")).toBe(false);
    expect(exportCsv).not.toHaveBeenCalled();
    expect(text(root)).not.toContain("归因报表");expect(text(root)).not.toContain("周期核对");
    const basicReads=fetcher.mock.calls.filter(call=>String(call[0]).includes('/reports/betting?')).length;
    report.mockRejectedValueOnce(new AdminApi.AdminApiError("Unavailable",503));
    clickHostButton(button(root,"查询"));await flush();
    expect(report).toHaveBeenLastCalledWith(brand,expect.objectContaining({agent_id:"55555555-5555-4555-8555-555555555555"}));
    expect(fetcher.mock.calls.filter(call=>String(call[0]).includes('/reports/betting?'))).toHaveLength(basicReads);
    expect(nodes(results).some(n=>String(n.props.class).includes('report-metrics'))).toBe(false);
  });
});
