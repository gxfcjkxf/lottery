import {test} from "node:test";
import assert from "node:assert/strict";
import {spawnSync} from "node:child_process";
import {assertCoverage,assertReferences,composeDocument,documentedRoutes} from "../../scripts/openapi-lib.mjs";
import * as identity from "../../docs/openapi/identity.mjs";
import * as finance from "../../docs/openapi/finance.mjs";
import * as lottery from "../../docs/openapi/lottery.mjs";
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
import * as attributionReports from "../../scripts/openapi-attribution-reports.mjs";
import * as businessInventory from "../../scripts/openapi-business-inventory.mjs";

const modules=[identity,finance,lottery,withdrawals,withdrawalReports,workbench,commissionPolicies,commissionCycles,commissionDiscovery,commissionPayments,commissionAdjustments,commissionCorrections,commissionReports,rechargeUser,rewards,rewardReports,reportArchives,reportArchiveTasks,reportArchivePolicy,attributionReports];
modules.push(businessInventory);
const go=process.env.LOTTERY_GO_BIN??"go";
const operation={method:"POST",path:"/api/v1/me/action",operationId:"performAction",summary:"Submit action",tag:"identity",auth:"user",idempotency:true,requestBody:{$ref:"#/components/schemas/EmptyObject"},data:{$ref:"#/components/schemas/EmptyObject"}};
const routes=[{method:"POST",path:"/api/v1/me/action"},{method:"POST",path:"/api/v1/b/{brandCode}/me/action"}];
test("expands canonical user operations and keeps exact registered coverage",()=>{const doc=composeDocument([{schemas:{},operations:[operation]}],routes);assert.equal(documentedRoutes(doc).length,2);assert.equal(doc.paths["/api/v1/b/{brandCode}/me/action"].post.operationId,"performActionByBrand");assert.ok(doc.paths["/api/v1/me/action"].post.parameters.some(p=>p.name==="Idempotency-Key"&&p.required));assert.ok(doc.paths["/api/v1/me/action"].post.parameters.some(p=>p.name==="Origin"&&!p.required));assert.deepEqual(doc.paths["/api/v1/me/action"].post.security,[{userBearer:[]}])});
test("rejects missing/invented operations rather than silently documenting future APIs",()=>{const doc=composeDocument([{schemas:{},operations:[operation]}],routes);assert.throws(()=>assertCoverage(doc,[...routes,{method:"POST",path:"/api/v1/withdrawals"}]),/Missing/);assert.throws(()=>assertCoverage(doc,routes.slice(0,1)),/Unregistered/)});
test("rejects duplicate schema/operation ids and unresolved/external refs",()=>{assert.throws(()=>composeDocument([{schemas:{UUID:{}},operations:[]}],[]),/Duplicate schema/);assert.throws(()=>composeDocument([{schemas:{},operations:[operation,operation]}],routes),/Duplicate/);assert.throws(()=>assertReferences({schema:{$ref:"#/components/schemas/Missing"}}),/Unresolved/);assert.throws(()=>assertReferences({schema:{$ref:"https://untrusted.example/schema"}}),/External/)});
test("does not add mutable brand authorization to global admin auth",()=>{const op={...operation,path:"/api/v1/admin/auth/logout",auth:"admin",operationId:"logoutAdmin",brandHeader:false};const doc=composeDocument([{schemas:{},operations:[op]}],[{method:"POST",path:op.path}]);assert.ok(!doc.paths[op.path].post.parameters.some(p=>p.name==="X-Brand-ID"));assert.deepEqual(doc.paths[op.path].post.security,[{adminBearer:[]},{adminCookie:[]}])});
test("fails duplicate headers and invalid authentication declarations",()=>{
  const op={...operation,path:"/api/v1/admin/action",auth:"admin",brandHeader:true,parameters:[{name:"X-Brand-ID",in:"header",schema:{type:"string"}}]};
  assert.throws(()=>composeDocument([{schemas:{},operations:[op]}],[{method:"POST",path:op.path}]),/Duplicate parameter/);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...operation,auth:"unknown"}]}],routes),/Invalid auth/);
});
test("composes the explicit API module list against real registered routes, including payments",()=>{
  const result=spawnSync(go,["run","-buildvcs=false","./cmd/route-inventory"],{cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}});
  assert.equal(result.status,0,result.stderr||result.error?.message);
  const routes=JSON.parse(result.stdout);
  const doc=composeDocument(modules,routes);
  assertCoverage(doc,routes);
  assertReferences(doc);
  assert.deepEqual(documentedRoutes(doc).filter(path=>path.includes("/commission-payment" )).sort(),[
    "GET /api/v1/admin/commission-payment-policy",
    "GET /api/v1/admin/commission-payments",
    "GET /api/v1/admin/commission-payments/{id}",
    "GET /api/v1/admin/commission-payments/{id}/targets",
    "GET /api/v1/admin/commission-payment-targets/{id}/adjustments",
    "POST /api/v1/admin/commission-payment-targets/{id}/adjustments",
    "POST /api/v1/admin/commission-payments/{id}/approve",
    "POST /api/v1/admin/commission-payments/{id}/retry",
    "PUT /api/v1/admin/commission-payment-policy",
  ].sort());
});
