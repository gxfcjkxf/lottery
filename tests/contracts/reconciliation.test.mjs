import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {spawnSync} from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {operations as identityOperations,schemas as identitySchemas} from "../../docs/openapi/identity.mjs";
import {operations,schemas} from "../../docs/openapi/finance.mjs";
import {operations as lotteryOperations,schemas as lotterySchemas} from "../../docs/openapi/lottery.mjs";
import {composeDocument} from "../../scripts/openapi-lib.mjs";
import * as workbench from "../../scripts/openapi-workbench.mjs";
import * as withdrawals from "../../scripts/openapi-withdrawal-orders.mjs";
import * as withdrawalReports from "../../scripts/openapi-withdrawal-reports.mjs";
import * as commissionPolicies from "../../scripts/openapi-commission-policies.mjs";
import * as commissionCycles from "../../scripts/openapi-commission-cycles.mjs";
import * as commissionDiscovery from "../../scripts/openapi-commission-discovery.mjs";
import * as commissionPayments from "../../scripts/openapi-commission-payments.mjs";
import * as commissionAdjustments from "../../scripts/openapi-commission-adjustments.mjs";
import * as commissionCorrections from "../../scripts/openapi-commission-corrections.mjs";
import * as commissionReports from "../../scripts/openapi-commission-reports.mjs";
import * as rechargeUser from "../../scripts/openapi-recharge-user.mjs";
import * as rewards from "../../scripts/openapi-rewards.mjs";
import * as rewardReports from "../../scripts/openapi-reward-reports.mjs";
import * as reportArchives from "../../scripts/openapi-report-archives.mjs";
import * as reportArchiveTasks from "../../scripts/openapi-report-archive-tasks.mjs";
import * as reportArchivePolicy from "../../scripts/openapi-report-archive-policy.mjs";

const root=new URL("../../",import.meta.url);
const go=process.env.LOTTERY_GO_BIN??"go";
const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
ajv.addFormat("int64",{type:"number",validate:Number.isInteger});
ajv.addFormat("nonnegative-int64-string",{type:"string",validate:value=>/^(0|[1-9][0-9]*)$/.test(value)&&BigInt(value)<=9223372036854775807n});
const components={...doc.components,schemas:{...doc.components.schemas,...identitySchemas,...schemas,...lotterySchemas}};
ajv.addSchema({$id:"urn:lottery:reconciliation-contract",components});
const validate=name=>ajv.compile({$ref:`urn:lottery:reconciliation-contract#/components/schemas/${name}`});
const methodsByPath=new Map(operations.map(operation=>[`${operation.method} ${operation.path.replace("/api/v1/admin","")}`,operation]));
const expectedRoutes=[
  "GET /api/v1/admin/reconciliations",
  "POST /api/v1/admin/reconciliations",
  "GET /api/v1/admin/reconciliations/{id}",
  "GET /api/v1/admin/reconciliations/{id}/targets",
  "POST /api/v1/admin/reconciliations/{id}/retry",
];

test("reconciliation contract documents exactly the five registered admin routes",()=>{
  const result=spawnSync(go,["run","-buildvcs=false","./cmd/route-inventory"],{cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}});
  assert.equal(result.status,0,result.stderr||result.error?.message);
  const actual=new Set(JSON.parse(result.stdout).map(({method,path})=>`${method} ${path}`));
  const fullDocument=composeDocument([
    {schemas:identitySchemas,operations:identityOperations},
    {schemas,operations},
    {schemas:lotterySchemas,operations:lotteryOperations},
    workbench,
    withdrawals,
    withdrawalReports,
    commissionPolicies,
    commissionCycles,
    commissionAdjustments,
    commissionCorrections,
    rewards,
    rewardReports,
    commissionReports,
    rechargeUser,
    commissionDiscovery,
    commissionPayments,
    reportArchives,
    reportArchiveTasks,
    reportArchivePolicy,
  ],JSON.parse(result.stdout));
  for(const route of expectedRoutes){
    assert.ok(actual.has(route),`backend route missing: ${route}`);
    assert.ok(methodsByPath.has(route.replace("/api/v1/admin","")),`OpenAPI route missing: ${route}`);
    assert.ok(fullDocument.paths[route.split(" ")[1]],`assembled OpenAPI path missing: ${route}`);
  }
  const reconciliationRoutes=[...methodsByPath.keys()].filter(key=>key.includes("/reconciliations"));
  assert.deepEqual(reconciliationRoutes.sort(),expectedRoutes.map(route=>route.replace("/api/v1/admin","")).sort());
});

test("reconciliation DTO examples satisfy closed legacy and modern schemas and preserve nulls and zero versions",()=>{
  for(const name of Object.keys(components.schemas))assert.doesNotThrow(()=>validate(name),`${name} schema compiles`);
  for(const name of ["FinanceReconciliationJob","FinanceReconciliationJobPage","FinanceReconciliationTarget","FinanceReconciliationTargetPage"]){
    assert.ok(validate(name),`${name} schema compiles`);
  }
  const result=spawnSync(go,["run","-buildvcs=false","./cmd/contract-examples"],{cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}});
  assert.equal(result.status,0,result.stderr||result.error?.message);
  const examples=JSON.parse(result.stdout);
  for(const name of ["FinanceReconciliationJob","FinanceReconciliationJobPage","FinanceReconciliationTarget","FinanceReconciliationTargetPage"]){
    const check=validate(name);
    assert.ok(check(examples[name]),`${name}: ${JSON.stringify(check.errors)}`);
  }
  const job=examples.FinanceReconciliationJob;
  assert.equal(job.version,1);
  assert.equal(job.target_count,"2");
  assert.equal(job.consistent_count,"1");
  assert.equal(job.completed_at,null);
  assert.equal(job.last_error_code,null);
  const [checked,pending]=examples.FinanceReconciliationTargetPage.items;
  assert.equal(job.check_scope,"wallet");
  assert.equal(checked.preview.version,0);
  assert.equal(checked.preview.ledger_version,0);
  assert.deepEqual(checked.preview.actual,{gift:{manual_frozen:"0"},recharge:{available:"11"}});
  assert.equal(checked.check_scope,"wallet");
  assert.equal(checked.business_preview,null);
  assert.equal(pending.check_scope,"wallet");
  assert.equal(pending.business_preview,null);
  assert.equal(pending.outcome,null);
  assert.equal(pending.preview,null);
  assert.equal(pending.error_code,null);
  assert.equal(pending.checked_at,null);
  assert.equal(pending.audit_log_id,null);

  const businessJob=examples.FinanceReconciliationJobModern;
  const businessTarget=examples.FinanceReconciliationTargetModern;
  assert.ok(validate("FinanceReconciliationJob")(businessJob));
  assert.ok(validate("FinanceReconciliationTarget")(businessTarget));
  assert.equal(businessJob.check_scope,"wallet_and_business");
  assert.equal(businessTarget.check_scope,"wallet_and_business");
  assert.equal(businessTarget.business_preview.account_version,0);
  assert.equal(businessTarget.business_preview.issues.length,0);
  assert.equal(businessTarget.business_preview.coverage.length,12);

  const legacyJob={...job};
  delete legacyJob.check_scope;
  assert.ok(validate("FinanceReconciliationJob")(legacyJob),"the old job shape remains readable");
  const legacyTarget={...checked};
  delete legacyTarget.check_scope;
  delete legacyTarget.business_preview;
  assert.ok(validate("FinanceReconciliationTarget")(legacyTarget),"the old target shape remains readable");
});

test("reconciliation permissions, primary reads, strict pagination, and idempotent writes match handler behavior",()=>{
  const list=methodsByPath.get("GET /reconciliations");
  const create=methodsByPath.get("POST /reconciliations");
  const detail=methodsByPath.get("GET /reconciliations/{id}");
  const targets=methodsByPath.get("GET /reconciliations/{id}/targets");
  const retry=methodsByPath.get("POST /reconciliations/{id}/retry");
  for(const operation of [list,detail,targets]){
    assert.deepEqual(operation.permissions,["wallet.view.brand","wallet.view.platform"]);
    assert.match(operation.description,/primary database/i);
    assert.ok(!(operation.parameters??[]).some(parameter=>/^x-read-/i.test(parameter.name)));
  }
  for(const operation of [create,retry]){
    assert.deepEqual(operation.permissions,["wallet.view.brand","wallet.view.platform","wallet.reconcile.brand"]);
    assert.equal(operation.idempotency,true);
    assert.match(operation.description,/Super-admins are read-only/);
    assert.match(operation.description,/explicit brand membership/);
  }
  assert.deepEqual(create.requestBody,{ $ref:"#/components/schemas/FinanceReconciliationCreateInput" });
  assert.deepEqual(schemas.FinanceReconciliationCreateInput.required,["reason"]);
  assert.deepEqual(Object.keys(schemas.FinanceReconciliationCreateInput.properties),["reason","check_scope"]);
  assert.deepEqual(schemas.FinanceReconciliationCreateInput.properties.check_scope.enum,["wallet","wallet_and_business"]);
  assert.equal(schemas.FinanceReconciliationCreateInput.properties.check_scope.default,"wallet");
  assert.deepEqual(targets.parameters.filter(parameter=>parameter.in==="query").map(parameter=>parameter.name),["limit","offset","outcome"]);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="limit").schema.default,20);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="limit").schema.maximum,100);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="offset").schema.maximum,1000000);
  assert.deepEqual(schemas.FinanceReconciliationTargetPage.properties.outcome.anyOf[0].enum,["pending","failed","consistent","repairable","corrupt"]);

  const routeSource=readFileSync(new URL("../../backend/internal/httpapi/reconciliations.go",import.meta.url),"utf8");
  const createInputSource=readFileSync(new URL("../../backend/internal/httpapi/reconciliation_input.go",import.meta.url),"utf8");
  const modelSource=readFileSync(new URL("../../backend/internal/reconciliation/model.go",import.meta.url),"utf8");
  const accessSource=readFileSync(new URL("../../backend/internal/access/access.go",import.meta.url),"utf8");
  assert.match(routeSource,/d\.Admins\.DB\.Begin\(ctx\)/);
  assert.match(routeSource,/reconciliation\.Allowed\(fresh, brand, "view"\)/);
  assert.match(routeSource,/r\.Header\.Get\("Idempotency-Key"\)/);
  for(const code of ["RECONCILIATION_INPUT_INVALID","RECONCILIATION_NOT_FOUND","RECONCILIATION_STATE_CONFLICT","RECONCILIATION_VERSION_CONFLICT","RECONCILIATION_TOO_LARGE"]){
    assert.ok(routeSource.includes(code),`real handler missing standard error code ${code}`);
  }
  assert.equal((routeSource.match(/d\.Mutations\.ExecuteChecked/g)??[]).length,2);
  assert.ok(routeSource.includes("var in reconciliationCreateInput"));
  for(const guard of ["utf8.Valid(raw)","fields[key] != nil","!allowed[key]","bytes.TrimSpace(value)","reconciliation.ValidScope(text)"]){
    assert.ok(createInputSource.includes(guard),`strict create decoder missing ${guard}`);
  }
  assert.match(modelSource,/return \(action == "run" \|\| action == "retry"\) && view && !a\.SuperAdmin && access\.Authorize\(a, "wallet", "reconcile", access\.ScopeBrand, brand\)/);
  assert.match(accessSource,/scope == ScopeBrand && \(brandID == "" \|\| !contains\(account\.BrandIDs, brandID\)\)/);
});

test("reconciliation schemas enforce canonical bounded versions, exact counters, and strict reasons",()=>{
  const epoch=ajv.compile(commissionCorrections.schemas.CommissionCorrectionPlan.properties.evidence_epoch);
  assert.ok(epoch("9223372036854775807"));
  assert.ok(!epoch("9223372036854775808"),"19 digits alone must not admit values above int64");
  assert.ok(!epoch("01"),"epoch decimal strings must be canonical unsigned integers");
  assert.ok(!epoch("-1"),"epoch decimal strings are unsigned");
  const versionSchema=schemas.FinanceReconciliationRetryInput.properties.version;
  assert.equal(versionSchema.minimum,1);
  assert.equal(versionSchema.maximum,Number.MAX_SAFE_INTEGER-1);
  assert.ok(validate("FinanceReconciliationRetryInput")({version:1,reason:"retry failed observation"}));
  assert.ok(!validate("FinanceReconciliationRetryInput")({version:0,reason:"retry"}));
  assert.ok(!validate("FinanceReconciliationRetryInput")({version:Number.MAX_SAFE_INTEGER,reason:"retry"}));
  assert.ok(!validate("FinanceReconciliationRetryInput")({version:Number.MAX_SAFE_INTEGER+1,reason:"retry"}));
  assert.ok(validate("FinanceReconciliationCreateInput")({reason:"wallet check"}));
  assert.ok(validate("FinanceReconciliationCreateInput")({reason:"wallet check",check_scope:"wallet"}));
  assert.ok(validate("FinanceReconciliationCreateInput")({reason:"business check",check_scope:"wallet_and_business"}));
  assert.ok(!validate("FinanceReconciliationCreateInput")({reason:"check",check_scope:null}));
  assert.ok(!validate("FinanceReconciliationCreateInput")({reason:"check",check_scope:"all"}));
  assert.ok(!validate("FinanceReconciliationCreateInput")({reason:"check",check_scope:"wallet",extra:true}));
  for(const reason of ["", " leading", "trailing ", "line\nbreak", "line\rbreak", "nul\u0000byte"]){
    assert.ok(!validate("FinanceReconciliationCreateInput")({reason}),`accepted invalid reason ${JSON.stringify(reason)}`);
  }
  const page=validate("FinanceReconciliationJobPage");
  const example={brand_id:"11111111-1111-4111-8111-111111111111",items:[],total_count:"10000000000000000000",limit:20,offset:0};
  assert.ok(page(example));
  assert.ok(!page({...example,total_count:10000000000000000000}));
  assert.ok(validate("FinanceReconciliationJob")({
    id:"11111111-1111-4111-8111-111111111111",brand_id:"11111111-1111-4111-8111-111111111111",state:"pending",version:1,
    target_count:"100000",checked_count:"0",consistent_count:"0",repairable_count:"0",corrupt_count:"0",failed_count:"0",pending_count:"100000",
    created_by:"11111111-1111-4111-8111-111111111111",reason:"check",created_at:"2026-10-06T00:00:00Z",started_at:null,completed_at:null,last_error_code:null,can_retry:false,creation_audit_log_id:"11111111-1111-4111-8111-111111111111",
  }));
  assert.ok(!validate("FinanceReconciliationJob")({
    id:"11111111-1111-4111-8111-111111111111",brand_id:"11111111-1111-4111-8111-111111111111",state:"pending",version:1,
    target_count:"100001",checked_count:"0",consistent_count:"0",repairable_count:"0",corrupt_count:"0",failed_count:"0",pending_count:"100001",
    created_by:"11111111-1111-4111-8111-111111111111",reason:"check",created_at:"2026-10-06T00:00:00Z",started_at:null,completed_at:null,last_error_code:null,can_retry:false,creation_audit_log_id:"11111111-1111-4111-8111-111111111111",
  }));
});

test("business previews are exact, unbounded-count snapshots and only full checked targets carry them",()=>{
  const businessJob={
    id:"11111111-1111-4111-8111-111111111111",brand_id:"11111111-1111-4111-8111-111111111111",state:"completed",version:2,
    target_count:"1",checked_count:"1",consistent_count:"1",repairable_count:"0",corrupt_count:"0",failed_count:"0",pending_count:"0",
    created_by:"11111111-1111-4111-8111-111111111111",reason:"check",created_at:"2026-10-06T00:00:00Z",started_at:null,completed_at:"2026-10-06T00:01:00Z",last_error_code:null,can_retry:false,creation_audit_log_id:"11111111-1111-4111-8111-111111111111",check_scope:"wallet_and_business",
  };
  assert.ok(validate("FinanceReconciliationJob")(businessJob));
  assert.ok(!validate("FinanceReconciliationJob")({...businessJob,check_scope:"wallet_and_business",unexpected:true}));

  const id="11111111-1111-4111-8111-111111111111";
  const counts="999999999999999999999999999999999999999999999999";
  const families=["bet","commission","commission_adjustment","commission_correction","manual","prize","prize_reversal","recharge","refund","reward","unknown","withdrawal"];
  const coverage=families.map(family=>({family,ledger_entry_count:counts,business_reference_count:"0",issue_count:"0"}));
  const preview={account_id:id,member_id:id,account_version:0,ledger_entry_count:counts,business_reference_count:"0",issue_count:"0",issues_truncated:false,consistent:true,fingerprint:"a".repeat(64),issues:[],coverage};
  const checked={id,brand_id:id,job_id:id,account_id:id,member_id:id,state:"checked",outcome:"consistent",preview:null,attempt_count:1,error_code:null,checked_at:"2026-10-06T00:01:00Z",audit_log_id:id,check_scope:"wallet_and_business",business_preview:preview};
  const checkTarget=validate("FinanceReconciliationTarget");
  assert.ok(checkTarget(checked),JSON.stringify(checkTarget.errors));
  assert.ok(!checkTarget({...checked,business_preview:null}),"checked full-scope targets require a preview");
  assert.ok(!checkTarget({...checked,state:"pending",business_preview:preview}),"nonterminal targets cannot carry a preview");
  assert.ok(!checkTarget({...checked,check_scope:"wallet",business_preview:preview}),"wallet-only targets cannot carry a business preview");
  assert.ok(!checkTarget({...checked,unexpected:true}),"modern target shape is closed");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,issue_count:"01"}}),"counts are canonical decimal strings");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,coverage:[...coverage].reverse()}}),"coverage families must be in exact sorted order");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,coverage:coverage.slice(1)}}),"all twelve coverage families are required");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,issues:Array.from({length:101},()=>({code:"MISSING_LEDGER_ENTRY",entry_type:null,ledger_entry_id:null,resource_type:"ledger",resource_id:null}))}}),"issue samples are capped at 100");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,extra:true}}),"preview schema is closed");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,account_version:Number.MAX_SAFE_INTEGER+1}}),"account versions are bounded safe integers");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,fingerprint:"z".repeat(64)}}),"fingerprints are hexadecimal");
  assert.ok(!checkTarget({...checked,business_preview:{...preview,issues:[{code:"MISSING_LEDGER_ENTRY",entry_type:"ledger-é",ledger_entry_id:null,resource_type:"ledger",resource_id:null}]}}));
  assert.ok(!checkTarget({...checked,business_preview:{...preview,issues:[{code:"OTHER",entry_type:null,ledger_entry_id:null,resource_type:"ledger",resource_id:null}]}}),"issue codes are closed");

  const issueSchema=validate("FinanceReconciliationBusinessIssue");
  for(const entry_type of ["UpperCase", "tag.with.dot", "tag:with:colon", "tag-with-hyphen", "A_9.:--"+"x".repeat(193)]){
    assert.ok(issueSchema({code:"UNSUPPORTED_LEDGER_TYPE",entry_type,ledger_entry_id:id,resource_type:"ledger",resource_id:id}),`accepted ASCII ledger tag ${entry_type}`);
  }
  for(const entry_type of ["x".repeat(201), "ledger-é"]){
    assert.ok(!issueSchema({code:"UNSUPPORTED_LEDGER_TYPE",entry_type,ledger_entry_id:id,resource_type:"ledger",resource_id:id}),`rejected invalid ledger tag ${entry_type}`);
  }
  const unsafeNumber="9007199254740992";
  const unsafeAggregate=(BigInt(unsafeNumber)*BigInt(families.length)).toString();
  assert.ok(checkTarget({...checked,business_preview:{...preview,business_reference_count:unsafeAggregate,
    coverage:coverage.map(row=>({...row,business_reference_count:unsafeNumber}))}}),"canonical aggregate decimal strings may exceed JavaScript's safe integer range");

  const wallet={...checked,check_scope:"wallet",business_preview:null};
  assert.ok(checkTarget(wallet));
  assert.ok(!checkTarget({...wallet,business_preview:preview}));
  const old={...wallet};
  delete old.check_scope;
  delete old.business_preview;
  assert.ok(checkTarget(old),"legacy target shape remains accepted");
});
