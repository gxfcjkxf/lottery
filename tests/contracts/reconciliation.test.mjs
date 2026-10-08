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
import * as commissionReports from "../../scripts/openapi-commission-reports.mjs";
import * as rechargeUser from "../../scripts/openapi-recharge-user.mjs";
import * as rewards from "../../scripts/openapi-rewards.mjs";
import * as rewardReports from "../../scripts/openapi-reward-reports.mjs";

const root=new URL("../../",import.meta.url);
const go=process.env.LOTTERY_GO_BIN??"go";
const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
ajv.addFormat("int64",{type:"number",validate:Number.isInteger});
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
    rewards,
    rewardReports,
    commissionReports,
    rechargeUser,
    commissionDiscovery,
    commissionPayments,
  ],JSON.parse(result.stdout));
  for(const route of expectedRoutes){
    assert.ok(actual.has(route),`backend route missing: ${route}`);
    assert.ok(methodsByPath.has(route.replace("/api/v1/admin","")),`OpenAPI route missing: ${route}`);
    assert.ok(fullDocument.paths[route.split(" ")[1]],`assembled OpenAPI path missing: ${route}`);
  }
  const reconciliationRoutes=[...methodsByPath.keys()].filter(key=>key.includes("/reconciliations"));
  assert.deepEqual(reconciliationRoutes.sort(),expectedRoutes.map(route=>route.replace("/api/v1/admin","")).sort());
});

test("reconciliation DTO examples satisfy closed schemas and preserve nulls, zero versions, and sparse actual balances",()=>{
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
  assert.equal(checked.preview.version,0);
  assert.equal(checked.preview.ledger_version,0);
  assert.deepEqual(checked.preview.actual,{gift:{manual_frozen:"0"},recharge:{available:"11"}});
  assert.equal(pending.outcome,null);
  assert.equal(pending.preview,null);
  assert.equal(pending.error_code,null);
  assert.equal(pending.checked_at,null);
  assert.equal(pending.audit_log_id,null);
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
  assert.deepEqual(Object.keys(schemas.FinanceReconciliationCreateInput.properties),["reason"]);
  assert.deepEqual(targets.parameters.filter(parameter=>parameter.in==="query").map(parameter=>parameter.name),["limit","offset","outcome"]);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="limit").schema.default,20);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="limit").schema.maximum,100);
  assert.equal(targets.parameters.find(parameter=>parameter.name==="offset").schema.maximum,1000000);
  assert.deepEqual(schemas.FinanceReconciliationTargetPage.properties.outcome.anyOf[0].enum,["pending","failed","consistent","repairable","corrupt"]);

  const routeSource=readFileSync(new URL("../../backend/internal/httpapi/reconciliations.go",import.meta.url),"utf8");
  const modelSource=readFileSync(new URL("../../backend/internal/reconciliation/model.go",import.meta.url),"utf8");
  const accessSource=readFileSync(new URL("../../backend/internal/access/access.go",import.meta.url),"utf8");
  assert.match(routeSource,/d\.Admins\.DB\.Begin\(ctx\)/);
  assert.match(routeSource,/reconciliation\.Allowed\(fresh, brand, "view"\)/);
  assert.match(routeSource,/r\.Header\.Get\("Idempotency-Key"\)/);
  for(const code of ["RECONCILIATION_INPUT_INVALID","RECONCILIATION_NOT_FOUND","RECONCILIATION_STATE_CONFLICT","RECONCILIATION_VERSION_CONFLICT","RECONCILIATION_TOO_LARGE"]){
    assert.ok(routeSource.includes(code),`real handler missing standard error code ${code}`);
  }
  assert.equal((routeSource.match(/d\.Mutations\.ExecuteChecked/g)??[]).length,2);
  assert.match(modelSource,/return \(action == "run" \|\| action == "retry"\) && view && !a\.SuperAdmin && access\.Authorize\(a, "wallet", "reconcile", access\.ScopeBrand, brand\)/);
  assert.match(accessSource,/scope == ScopeBrand && \(brandID == "" \|\| !contains\(account\.BrandIDs, brandID\)\)/);
});

test("reconciliation schemas enforce canonical bounded versions, exact counters, and strict reasons",()=>{
  const versionSchema=schemas.FinanceReconciliationRetryInput.properties.version;
  assert.equal(versionSchema.minimum,1);
  assert.equal(versionSchema.maximum,Number.MAX_SAFE_INTEGER);
  assert.ok(validate("FinanceReconciliationRetryInput")({version:1,reason:"retry failed observation"}));
  assert.ok(!validate("FinanceReconciliationRetryInput")({version:0,reason:"retry"}));
  assert.ok(!validate("FinanceReconciliationRetryInput")({version:Number.MAX_SAFE_INTEGER+1,reason:"retry"}));
  assert.ok(validate("FinanceReconciliationCreateInput")({reason:"wallet check"}));
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
