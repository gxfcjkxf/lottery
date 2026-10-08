import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { createRenderer, h, nextTick, ref, type Component } from "vue";
import * as VueRuntime from "vue";
import * as AdminApi from "./admin-api";
import { AdminApiError } from "./admin-api";
import * as AdminI18n from "./i18n";
import { adminI18nKey, createAdminI18n } from "./i18n";
import type { AdminAccount } from "./admin-api";
import * as CorrectionApi from "./commission-corrections-api";
import * as CorrectionState from "./commission-corrections-state";
import * as PaymentApi from "./commissionPayments-api";

const brand = "11111111-1111-4111-8111-111111111111", actor = "33333333-3333-4333-8333-333333333333";
const planId = "44444444-4444-4444-8444-444444444444", executionId = "55555555-5555-4555-8555-555555555555";
const cycleId = "66666666-6666-4666-8666-666666666666", paymentId = "77777777-7777-4777-8777-777777777777";
const runId = "88888888-8888-4888-8888-888888888888", auditId = "99999999-9999-4999-8999-999999999999";
const timestamp = "2026-10-07T00:00:00Z";
const privileges = ["commission.view.brand", "commission_correction_policy.write.brand", "commission_correction.retry.brand", "commission_correction.approve.brand", "commission_correction.continue.brand", "commission_correction.execute_retry.brand"];
const baseAccount: AdminAccount = { id: actor, super_admin: false, brand_ids: [brand], permissions: [], permissions_by_brand: { [brand]: privileges } };
const calls: Array<{ method: string; args: unknown[] }> = [];
let policyValue: any, planValue: any, executionValue: any, planTargetValue: any, executionTargetValue: any;
let nextWrite: (() => Promise<unknown>) | null = null, failRefresh = false, delayPlan: ReturnType<typeof deferred<any>> | null = null, delayExecutionTargets: ReturnType<typeof deferred<any>> | null = null, executionTargetCount = "1";

function freshData() {
  policyValue = { brand_id: brand, version: 1, enabled: false, audit_log_id: "", updated_at: timestamp };
  planValue = { id: planId, brand_id: brand, cycle_id: cycleId, payment_id: paymentId, run_id: runId, state: "ready", payout_mode: "manual", version: 3, evidence_epoch: "8", before_points: "10", calculated_points: "8", credit_points: "0", debit_points: "2", net_points: "-2", target_count: "1", planned_count: "1", creation_audit_log_id: auditId, last_audit_log_id: auditId, last_error_code: null, created_at: timestamp, updated_at: timestamp };
  executionValue = { id: executionId, brand_id: brand, cycle_id: cycleId, plan_id: planId, run_id: runId, payout_mode: "manual", state: "awaiting_approval", version: 1, plan_version: 3, evidence_epoch: "8", credit_points: "0", debit_points: "2", net_points: "-2", applied_credit_points: "0", applied_debit_points: "0", target_count: "1", applied_count: "0", paused_plan_target_id: null, last_error_code: null, creation_audit_log_id: auditId, last_audit_log_id: auditId, approved_by: null, approval_actor_type: null, approval_audit_log_id: null, cycle_hold_active: false, created_at: timestamp, updated_at: timestamp };
  planTargetValue = { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", brand_id: brand, plan_id: planId, agent_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", member_id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", original_target_id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", earning_id: null, adjustment_version: 2, previous_correction_target_id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", financial_version: 4, points_before: "10", points_after: "8", delta_points: "-2", creation_audit_log_id: auditId, created_at: timestamp };
  executionTargetValue = { id: "ffffffff-ffff-4fff-8fff-ffffffffffff", brand_id: brand, execution_id: executionId, plan_target_id: planTargetValue.id, agent_id: planTargetValue.agent_id, member_id: planTargetValue.member_id, points_before: "10", points_after: "8", delta_points: "-2", state: "pending", ledger_entry_id: null, audit_log_id: null, financial_version: null, created_at: timestamp, applied_at: null };
  calls.length = 0; nextWrite = null; failRefresh = false; delayPlan = null; delayExecutionTargets = null; executionTargetCount = "1";
}
function page<T>(items: T[], brandId = brand, offset = 0) { return { brand_id: brandId, items, total_count: String(items.length), limit: 20, offset }; }
function createApi() {
  return {
    getPolicy: async (...args: unknown[]) => { calls.push({ method: "getPolicy", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); return policyValue; },
    updatePolicy: async (...args: unknown[]) => { calls.push({ method: "updatePolicy", args }); if (nextWrite) return nextWrite(); return { ...policyValue, version: 2, enabled: true, audit_log_id: auditId }; },
    listPlans: async (...args: unknown[]) => { calls.push({ method: "listPlans", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); if (delayPlan) return delayPlan.promise; return page([planValue], String(args[0]), Number(args[2])); },
    readPlan: async (...args: unknown[]) => { calls.push({ method: "readPlan", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); return planValue; },
    listPlanTargets: async (...args: unknown[]) => { calls.push({ method: "listPlanTargets", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); return { brand_id: brand, plan_id: planId, items: [planTargetValue], total_count: "1", limit: 100, offset: 0 }; },
    retryPlan: async (...args: unknown[]) => { calls.push({ method: "retryPlan", args }); if (nextWrite) return nextWrite(); return planValue; },
    listExecutions: async (...args: unknown[]) => { calls.push({ method: "listExecutions", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); return page([executionValue], String(args[0]), Number(args[2])); },
    readExecution: async (...args: unknown[]) => { calls.push({ method: "readExecution", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); return executionValue; },
    listExecutionTargets: async (...args: unknown[]) => { calls.push({ method: "listExecutionTargets", args }); if (failRefresh) throw new AdminApiError("GET failed", 503); if (delayExecutionTargets && Number(args[3]) > 0) return delayExecutionTargets.promise; return { brand_id: brand, execution_id: executionId, items: [executionTargetValue], total_count: executionTargetCount, limit: 100, offset: Number(args[3]) }; },
    approve: async (...args: unknown[]) => { calls.push({ method: "approve", args }); if (nextWrite) return nextWrite(); return executionValue; },
    continue: async (...args: unknown[]) => { calls.push({ method: "continue", args }); if (nextWrite) return nextWrite(); return executionValue; },
    retry: async (...args: unknown[]) => { calls.push({ method: "retry", args }); if (nextWrite) return nextWrite(); return executionValue; },
  };
}
const api = createApi();
vi.spyOn(CorrectionApi, "createCommissionCorrectionsApi").mockReturnValue(api as never);
vi.spyOn(PaymentApi, "createCommissionPaymentsApi").mockReturnValue({ read: async (...args: unknown[]) => { calls.push({ method: "readOriginalPayment", args }); return { run_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" }; } } as never);
vi.spyOn(CorrectionApi, "commissionCorrectionPermissions").mockImplementation((account: AdminAccount, brandId: string) => {
  const listed = account.permissions_by_brand?.[brandId] ?? [];
  const member = account.brand_ids.includes(brandId) && !account.super_admin;
  const view = account.permissions_by_brand?.[brandId]?.includes("commission.view.brand") ?? false;
  return { view, policyWrite: Boolean(member && view && listed.includes("commission_correction_policy.write.brand")), planRetry: Boolean(member && view && listed.includes("commission_correction.retry.brand")), approve: Boolean(member && view && listed.includes("commission_correction.approve.brand")), continue: Boolean(member && view && listed.includes("commission_correction.continue.brand")), retry: Boolean(member && view && listed.includes("commission_correction.execute_retry.brand")) };
});

type HostNode = { tag: string; props: Record<string, unknown>; children: HostNode[]; text: string; parent?: HostNode; value?: string; checked?: boolean; addEventListener: (...args: unknown[]) => void; removeEventListener: (...args: unknown[]) => void; getRootNode: () => HostNode; options: HostNode[] };
function element(tag: string): HostNode { return { tag, props: {}, children: [], text: "", value: "", checked: false, addEventListener() {}, removeEventListener() {}, getRootNode() { return this; }, get options() { return this.children.filter((child) => child.tag === "option"); } }; }
const renderer = createRenderer<HostNode, HostNode>({ createElement: element, createText: (text) => ({ ...element("#text"), text }), createComment: (text) => ({ ...element("#comment"), text }), setText: (node,text) => { node.text=text; }, setElementText: (node,text) => { node.text=text; node.children=[]; }, patchProp: (node,key,_old,value) => { node.props[key]=value; if(key==="value") node.value=String(value??""); }, insert: (node,parent,anchor) => { if(node.parent){const i=node.parent.children.indexOf(node);if(i>=0)node.parent.children.splice(i,1);} node.parent=parent;const i=anchor?parent.children.indexOf(anchor):-1;if(i<0)parent.children.push(node);else parent.children.splice(i,0,node); }, remove: (node) => {if(!node.parent)return;const i=node.parent.children.indexOf(node);if(i>=0)node.parent.children.splice(i,1);node.parent=undefined;}, parentNode: (node) => node.parent??null, nextSibling: (node) => node.parent?node.parent.children[node.parent.children.indexOf(node)+1]??null:null });
function compileComponent(): Component {
  const source = readFileSync(new URL("./CommissionCorrectionsManagement.vue", import.meta.url), "utf8");
  const descriptor = parseVue(source, { filename: "CommissionCorrectionsManagement.vue" }).descriptor;
  const compiled = compileVueScript(descriptor, { id: "commission-corrections-component-test", inlineTemplate: true }).content;
  const javascript = ts.transpileModule(compiled, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } }).outputText;
  const modules: Record<string, unknown> = { vue: VueRuntime, "./admin-api": AdminApi, "./i18n": AdminI18n, "./commission-corrections-api": CorrectionApi, "./commission-corrections-state": CorrectionState, "./commissionPayments-api": PaymentApi };
  const body = javascript.replace(/^import\s+\{([\s\S]*?)\}\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m, bindings: string, specifier: string) => { if (!(specifier in modules)) throw new Error(`Unmapped import ${specifier}`); return `const {${bindings.replace(/\s+as\s+/g, ": ")}}=__modules[${JSON.stringify(specifier)}];`; }).replace(/^import\s+(\w+)\s+from\s+["']([^"']+)["'];?\s*$/gm, (_m,binding: string,specifier: string)=>{if(!(specifier in modules))throw new Error(`Unmapped import ${specifier}`);return `const ${binding}=__modules[${JSON.stringify(specifier)}].default;`;}).replace(/export\s+default\s+/, "return ");
  return new Function("__modules",body)(modules) as Component;
}
import { compileScript as compileVueScript, parse as parseVue } from "vue/compiler-sfc";
import ts from "typescript";
const ComponentUnderTest = compileComponent();
function textOf(node: HostNode): string { return node.text + node.children.map(textOf).join(""); }
let lastRoot: HostNode | null = null;
function findNode(node: HostNode, match: (n: HostNode) => boolean): HostNode | null { if(match(node))return node;for(const child of node.children){const found=findNode(child,match);if(found)return found;}return null; }
function byTestId(root: HostNode,id: string): HostNode|null{return findNode(root,(node)=>node.props["data-testid"]===id);}
function button(root: HostNode,text: string):HostNode|null{return findNode(root,(node)=>node.tag==="button"&&textOf(node).includes(text));}
function click(node:HostNode|null){expect(node,`button was missing in: ${lastRoot?textOf(lastRoot):""}`).not.toBeNull();(node!.props.onClick as (()=>void)|undefined)?.();}
function setModel(node:HostNode|null,value:unknown){expect(node).not.toBeNull();(node!.props["onUpdate:modelValue"] as ((next:unknown)=>void)|undefined)?.(value);}
function setChecked(node:HostNode|null,value:boolean){expect(node).not.toBeNull();(node!.props.onChange as ((event:unknown)=>void)|undefined)?.({target:{checked:value}});}
function deferred<T>(){let resolve!:(value:T)=>void,reject!:(reason?:unknown)=>void;const promise=new Promise<T>((done,fail)=>{resolve=done;reject=fail;});return{promise,resolve,reject};}
async function flush(){for(let i=0;i<16;i++)await Promise.resolve();await nextTick();}
function mount(account=baseAccount,brandId=brand,status="active") { vi.stubGlobal("Document",class {});vi.stubGlobal("ShadowRoot",class {});const scope=ref({account,brandId,brandStatus:status}),container=element("root"),emitted:string[]=[],i18n=createAdminI18n();lastRoot=container;const app=renderer.createApp({setup:()=>()=>h(ComponentUnderTest,{...scope.value,onSessionInvalid:()=>emitted.push("session-invalid")})});app.provide(adminI18nKey,i18n);app.mount(container);return{app,scope,container,emitted,i18n}; }
afterEach(()=>{CorrectionState.clearAllCommissionCorrectionIntents();calls.length=0;failRefresh=false;nextWrite=null;delayPlan=null;delayExecutionTargets=null;executionTargetCount="1";vi.clearAllMocks();vi.stubGlobal("Document",class {});vi.stubGlobal("ShadowRoot",class {});});

describe("CommissionCorrectionsManagement",()=>{
  beforeEach(freshData);
  it("shows a separate default-off gate, frozen plan versus applied execution, provenance, inherited hold and zero delta",async()=>{
    executionValue={...executionValue,state:"paused",cycle_hold_active:true,approved_by:actor,approval_actor_type:"admin",approval_audit_log_id:auditId,applied_count:"1",applied_debit_points:"2"};
    executionTargetValue={...executionTargetValue,state:"applied",delta_points:"0",ledger_entry_id:null,audit_log_id:auditId,financial_version:5};
    const mounted=mount(baseAccount,brand,"paused");await flush();
    expect(textOf(mounted.container)).toContain("真实资金执行已关闭");expect(textOf(mounted.container)).toContain("OPEN-117");
    click(button(mounted.container,planId));await flush();click(button(mounted.container,executionId));await flush();
    expect(textOf(mounted.container)).toContain("跨run代次继承");expect(textOf(mounted.container)).toContain(auditId);expect(textOf(mounted.container)).toContain("零差额已见证");setModel(byTestId(mounted.container,"cc-execution-reason"),"Finance reviewed");await flush();
    expect((byTestId(mounted.container,"cc-continue")?.props.disabled)).toBe(false);expect((byTestId(mounted.container,"cc-approve")?.props.disabled)).toBe(true);
    expect(readFileSync(new URL("./CommissionCorrectionsManagement.vue",import.meta.url),"utf8")).toContain("@media(max-width:700px)");
    mounted.app.unmount();
  });
  it("requires complete approval provenance and an actively held cycle before continue",async()=>{
    executionValue={...executionValue,state:"paused",cycle_hold_active:false,approved_by:actor,approval_actor_type:"admin",approval_audit_log_id:auditId};
    const mounted=mount();await flush();click(button(mounted.container,executionId));await flush();setModel(byTestId(mounted.container,"cc-execution-reason"),"Continue only if currently held");await flush();
    expect(byTestId(mounted.container,"cc-continue")?.props.disabled).toBe(true);
    executionValue={...executionValue,cycle_hold_active:true,approval_audit_log_id:""};mounted.scope.value={...mounted.scope.value,brandStatus:"paused"};await flush();click(button(mounted.container,executionId));await flush();
    expect(byTestId(mounted.container,"cc-continue")?.props.disabled).toBe(true);mounted.app.unmount();
  });
  it("disables stale actions while a detail/target read is pending",async()=>{
    const targetRead=deferred<any>();executionTargetCount="200";
    const mounted=mount();await flush();click(button(mounted.container,executionId));await flush();delayExecutionTargets=targetRead;click(button(mounted.container,"下一页目标"));await flush();
    expect(byTestId(mounted.container,"cc-continue")?.props.disabled).toBe(true);expect(byTestId(mounted.container,"cc-approve")?.props.disabled).toBe(true);
    targetRead.resolve({brand_id:brand,execution_id:executionId,items:[executionTargetValue],total_count:"200",limit:100,offset:100});await flush();delayExecutionTargets=null;mounted.app.unmount();
  });
  it("requires explicit review before write, preserves an unknown intent across unmount, and replays identical request only after review",async()=>{
    const first=mount();await flush();click(button(first.container,executionId));await flush();
    setModel(byTestId(first.container,"cc-execution-reason"),"Approve correction generation");await flush();click(byTestId(first.container,"cc-approve"));await flush();
    expect(calls.filter((call)=>call.method==="approve")).toHaveLength(0);
    const checkbox=byTestId(first.container,"cc-confirm-check");expect(checkbox).not.toBeNull();
    nextWrite=async()=>{throw new TypeError("response lost");};setChecked(checkbox,true);await flush();click(byTestId(first.container,"cc-submit"));await flush();
    expect(textOf(first.container)).toContain("提交结果未知");expect(calls.filter((call)=>call.method==="approve")).toHaveLength(1);
    first.scope.value={...first.scope.value,account:{...baseAccount,permissions_by_brand:{[brand]:["commission.view.brand"]}}};await flush();expect(button(first.container,"查看原请求并重试")?.props.disabled).toBe(true);expect(byTestId(first.container,"cc-policy-review")?.props.disabled).toBe(true);
    first.scope.value={...first.scope.value,account:baseAccount};await flush();expect(button(first.container,"查看原请求并重试")?.props.disabled).toBe(false);expect(calls.filter((call)=>call.method==="approve")).toHaveLength(1);first.app.unmount();
    nextWrite=async()=>({ ...executionValue,state:"applying",version:2 });
    const second=mount();await flush();expect(textOf(second.container)).toContain("待人工复核");
    click(button(second.container,"查看原请求并重试"));await flush();const before=textOf(byTestId(second.container,"cc-review")!);setChecked(byTestId(second.container,"cc-confirm-check"),true);await flush();click(byTestId(second.container,"cc-submit"));await flush();
    const retries=calls.filter((call)=>call.method==="approve");expect(retries).toHaveLength(2);expect(retries[0].args.slice(1)).toEqual(retries[1].args.slice(1));expect(textOf(second.container)).toContain("操作回执（服务器 ACK）");expect(before).toContain("Approve correction generation");second.app.unmount();
  });
  it("keeps a success ACK as a receipt when follow-up GET fails and never resends",async()=>{
    const mounted=mount();await flush();click(button(mounted.container,executionId));await flush();setModel(byTestId(mounted.container,"cc-execution-reason"),"Approve once");await flush();click(byTestId(mounted.container,"cc-approve"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();
    nextWrite=async()=>({ ...executionValue,state:"applying",version:2 });failRefresh=true;click(byTestId(mounted.container,"cc-submit"));await flush();
    expect(byTestId(mounted.container,"cc-receipt")).not.toBeNull();expect(textOf(mounted.container)).toContain("GET failed");expect(textOf(mounted.container)).not.toContain("结果未知");expect(byTestId(mounted.container,"cc-execution-detail")).toBeNull();expect(calls.filter((call)=>call.method==="approve")).toHaveLength(1);
    mounted.i18n.setLocale("en");await flush();expect(textOf(mounted.container)).toContain("The server confirmed the operation receipt.");expect(textOf(mounted.container)).not.toContain("服务器已确认操作回执");expect(calls.filter((call)=>call.method==="approve")).toHaveLength(1);mounted.app.unmount();
  });
  it("retains a 409 until every related query succeeds and human discard is checked",async()=>{
    const mounted=mount();await flush();setModel(byTestId(mounted.container,"cc-reason"),"Enable gate");await flush();setModel(byTestId(mounted.container,"cc-policy-toggle"),true);await flush();click(byTestId(mounted.container,"cc-policy-review"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();nextWrite=async()=>{throw new AdminApiError("conflict",409);};click(byTestId(mounted.container,"cc-submit"));await flush();
    expect(byTestId(mounted.container,"cc-conflict")).not.toBeNull();failRefresh=true;click(button(byTestId(mounted.container,"cc-conflict")!,"刷新所有相关状态"));await flush();expect(button(mounted.container,"确认丢弃 409 原请求")?.props.disabled).toBe(true);
    failRefresh=false;click(button(byTestId(mounted.container,"cc-conflict")!,"刷新所有相关状态"));await flush();const check=findNode(byTestId(mounted.container,"cc-conflict")!,n=>n.tag==="input");setChecked(check,true);await flush();click(button(mounted.container,"确认丢弃 409 原请求"));await flush();expect(byTestId(mounted.container,"cc-conflict")).toBeNull();mounted.app.unmount();
  });
  it("separates plan retry from execution retry and blocks super-admin writes",async()=>{
    const noExec={...baseAccount,permissions_by_brand:{[brand]:["commission.view.brand","commission_correction.retry.brand"]}};const mounted=mount(noExec);await flush();
    planValue={...planValue,state:"failed",last_error_code:"PLAN_FAILED"};await flush();click(button(mounted.container,planId));await flush();setModel(byTestId(mounted.container,"cc-plan-reason"),"Retry preparation");await flush();expect(byTestId(mounted.container,"cc-plan-retry")?.props.disabled).toBe(false);expect(byTestId(mounted.container,"cc-execute-retry")).toBeNull();
    mounted.scope.value={...mounted.scope.value,account:{...baseAccount,super_admin:true}};await flush();click(button(mounted.container,planId));await flush();expect(byTestId(mounted.container,"cc-plan-retry")?.props.disabled).toBe(true);mounted.app.unmount();
  });
  it("clears a definitive 400 request without retaining an unknown intent",async()=>{
    const mounted=mount();await flush();click(button(mounted.container,executionId));await flush();setModel(byTestId(mounted.container,"cc-execution-reason"),"Reject bad version");await flush();click(byTestId(mounted.container,"cc-approve"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();nextWrite=async()=>{throw new AdminApiError("Invalid request",400);};click(byTestId(mounted.container,"cc-submit"));await flush();
    expect(byTestId(mounted.container,"cc-unknown")).toBeNull();expect(CorrectionState.listPendingCommissionCorrectionIntents(actor,brand)).toEqual([]);expect(textOf(mounted.container)).toContain("Invalid request");mounted.app.unmount();
  });
  it("clears all in-memory intents and emits session-invalid on 401",async()=>{
    const mounted=mount();await flush();setModel(byTestId(mounted.container,"cc-reason"),"Update policy");await flush();setModel(byTestId(mounted.container,"cc-policy-toggle"),true);await flush();click(byTestId(mounted.container,"cc-policy-review"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();nextWrite=async()=>{throw new AdminApiError("Session expired",401);};click(byTestId(mounted.container,"cc-submit"));await flush();
    expect(mounted.emitted).toContain("session-invalid");expect(CorrectionState.listPendingCommissionCorrectionIntents(actor,brand)).toEqual([]);mounted.app.unmount();
  });
  it("retains a matching deferred 409 across exit and return without exposing it in an unauthorized scope",async()=>{
    const delayed=deferred<unknown>();const mounted=mount();await flush();setModel(byTestId(mounted.container,"cc-reason"),"Enable gate");await flush();setModel(byTestId(mounted.container,"cc-policy-toggle"),true);await flush();click(byTestId(mounted.container,"cc-policy-review"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();nextWrite=()=>delayed.promise;click(byTestId(mounted.container,"cc-submit"));await flush();
    mounted.scope.value={...mounted.scope.value,account:{...baseAccount,permissions_by_brand:{[brand]:[]}}};await flush();delayed.reject(new AdminApiError("late conflict",409));await flush();
    expect(byTestId(mounted.container,"cc-conflict")).toBeNull();expect(textOf(mounted.container)).toContain("没有此品牌的佣金查看权限");
    mounted.scope.value={...mounted.scope.value,account:baseAccount};await flush();expect(byTestId(mounted.container,"cc-conflict")).not.toBeNull();expect(CorrectionState.listPendingCommissionCorrectionIntents(actor,brand)).toHaveLength(1);expect(calls.filter((call)=>call.method==="updatePolicy")).toHaveLength(1);mounted.app.unmount();
  });
  it("does not let a delayed old-session 401 clear a new session's pending intent",async()=>{
    const delayed=deferred<unknown>();const mounted=mount();await flush();setModel(byTestId(mounted.container,"cc-reason"),"Old session write");await flush();setModel(byTestId(mounted.container,"cc-policy-toggle"),true);await flush();click(byTestId(mounted.container,"cc-policy-review"));await flush();setChecked(byTestId(mounted.container,"cc-confirm-check"),true);await flush();nextWrite=()=>delayed.promise;click(byTestId(mounted.container,"cc-submit"));await flush();
    CorrectionState.clearAllCommissionCorrectionIntents();const replacement=CorrectionState.createCommissionCorrectionIntent({actorId:actor,brandId:brand,operation:"policy"},{version:9,enabled:false,reason:"New login pending request"},"new-session-key-0001");expect(replacement).not.toBeNull();CorrectionState.setPendingCommissionCorrectionIntent({actorId:actor,brandId:brand,operation:"policy"},replacement!);
    delayed.reject(new AdminApiError("old session expired",401));await flush();
    expect(CorrectionState.listPendingCommissionCorrectionIntents(actor,brand).map((intent)=>intent.key)).toEqual(["new-session-key-0001"]);expect(mounted.emitted).not.toContain("session-invalid");mounted.app.unmount();
  });
  it("ignores late prior-brand/account responses and keeps permission read-only",async()=>{
    const delayed=deferred<any>();delayPlan=delayed;const mounted=mount();await flush();mounted.scope.value={...mounted.scope.value,brandId:"22222222-2222-4222-8222-222222222222",account:{...baseAccount,id:"88888888-8888-4888-8888-888888888888",brand_ids:[],permissions_by_brand:{}}};await flush();delayPlan=null;delayed.resolve(page([{...planValue,id:"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}],brand));await flush();expect(textOf(mounted.container)).toContain("没有此品牌的佣金查看权限");expect(textOf(mounted.container)).not.toContain("10 / 8");expect(calls.filter((call)=>["approve","continue","retry","retryPlan","updatePolicy"].includes(call.method))).toHaveLength(0);mounted.app.unmount();
  });
  it("uses locale provider translations and keeps locale reactive",async()=>{
    const mounted=mount();await flush();expect(textOf(mounted.container)).toContain("佣金更正差额");mounted.i18n.setLocale("en");await flush();expect(textOf(mounted.container)).toContain("Commission correction");mounted.app.unmount();
  });
});
