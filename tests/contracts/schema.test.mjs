import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {spawnSync} from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {operations as lotteryOperations,schemas as lotterySchemas} from "../../docs/openapi/lottery.mjs";

const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
// OpenAPI annotates integers as int64; exact financial integers use strings.
ajv.addFormat("int64", {type:"number",validate:Number.isInteger});
const components={...doc.components,schemas:{...doc.components.schemas,...lotterySchemas}};
const root={$id:"urn:lottery:implemented-api",components};
ajv.addSchema(root);
const validate=schema=>ajv.compile({$ref:`urn:lottery:implemented-api#/components/schemas/${schema}`});
const exampleSchemaNames={
  WithdrawalOrderExample:"WithdrawalOrder",
  WithdrawalOrderPageExample:"WithdrawalOrderPage",
  WithdrawalHistoryExample:"WithdrawalHistory",
  WithdrawalAvailabilityExample:"WithdrawalAvailability",
  WithdrawalReportExample:"WithdrawalReport",
};

test("every component and operation body is a compilable JSON Schema",()=>{
  for(const name of Object.keys(components.schemas))validate(name);
  const bodies=[];
  for(const item of Object.values(doc.paths))for(const operation of Object.values(item))bodies.push(operation);
  for(const operation of lotteryOperations)bodies.push(operation);
  for(const operation of bodies){
    const schemas=[operation.requestBody?.content?.["application/json"]?.schema,...Object.values(operation.responses??{}).flatMap(r=>Object.values(r.content??{}).map(media=>media.schema))].filter(Boolean);
    if(operation.requestBody&&!operation.requestBody.content)schemas.push(operation.requestBody);
    for(const schema of schemas)ajv.compile({...schema,components});
  }
});
test("actual Go DTO serialization and rule-engine outputs satisfy contracts",()=>{
  const result=spawnSync(process.env.LOTTERY_GO_BIN??"go",["run","-buildvcs=false","./cmd/contract-examples"],{cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}});
  assert.equal(result.status,0,result.stderr||result.error?.message);
  const examples=JSON.parse(result.stdout);
  assert.ok(Object.keys(examples).length>=15);
  for(const [name,value] of Object.entries(examples)){
    const schemaName=exampleSchemaNames[name]??(name==="AdminWorkbench"?"AdminWorkbenchSnapshot":name.replace(/Sparse$|Snapshot$/, ""));
    const check=validate(schemaName);assert.ok(check(value),`${name} (${schemaName}): ${JSON.stringify(check.errors)}`);
  }
  assert.equal(examples.AdminWorkbench.withdrawals.status,"ready");
  assert.equal(examples.AdminWorkbench.withdrawals.data.processing_points,"9000000000000000000");
  assert.equal(examples.AdminWorkbench.commissions.data,null);
  assert.equal(examples.AdminWorkbench.rewards.data,null);
  assert.equal(examples.LotterySimulationResult.bet_points,"8");
  assert.equal(examples.LotterySimulationResult.prize_points,"70");
  assert.equal(examples.WithdrawalAvailabilityExample.real_payments,false);
  assert.match(examples.WithdrawalAvailabilityExample.actor_context,/^[0-9a-f]{64}$/);
});
test("financial syntax stays exact, negative deltas differ from balances, and requests are closed",()=>{
  assert.ok(validate("Int64String")("-8"));assert.ok(!validate("Int64String")("-0"));
  assert.ok(!validate("NonnegativeInt64String")("-8"));assert.ok(!validate("PositiveInt64String")(8));
  assert.ok(!validate("FinanceAdjust")({source:"recharge",delta:"0",reason:"test"}));
  assert.ok(!validate("FinanceAdjust")({source:"recharge",delta:"-9223372036854775808",reason:"test"}));
  assert.ok(!validate("FinanceAdjust")({source:"recharge",delta:"8",reason:"test",unexpected:true}));
  assert.ok(validate("FinanceAdjust")({source:"recharge",delta:"8",reason:"test"}));
});
test("full-replacement finance requests require explicit nullable fields",()=>{
  const id="11111111-1111-4111-8111-111111111111";
  const input={policy_version:1,member_id:id,parent_id:null,parent_version:null,config:{ratio:"0",mode:null,status:"active",can_create_children:false},reason:"test"};
  assert.ok(validate("FinanceAgentCreateInput")(input));
  const missingParent=structuredClone(input);delete missingParent.parent_id;
  assert.ok(!validate("FinanceAgentCreateInput")(missingParent));
  assert.ok(validate("FinanceWithdrawalGameConfig")({turnover_multiple:null}));
  assert.ok(!validate("FinanceWithdrawalGameConfig")({}));
});

test("new withdrawal policy writes require positive N while legacy zero history stays readable",()=>{
  const check=validate("FinanceWithdrawalPositiveMultiple");
  for(const value of ["1","0.000001","0.25","2.5","999999.999999","1000000"])
    assert.ok(check(value),`${value}: ${JSON.stringify(check.errors)}`);
  for(const value of ["0","01","1.0","1e2","-1","0.0000001","1000000.000001","1000001"])
    assert.ok(!check(value),`invalid new N accepted: ${value}`);
  assert.ok(validate("FinanceWithdrawalGameConfig")({turnover_multiple:"0"}));
  assert.ok(!validate("FinanceWithdrawalGameWriteConfig")({turnover_multiple:"0"}));
  assert.ok(validate("FinanceWithdrawalGameWriteConfig")({turnover_multiple:null}));
  assert.ok(validate("FinanceWithdrawalGameWriteConfig")({turnover_multiple:"0.000001"}));
  const brand={enabled:false,min_points:"1",max_points:null,allowed_sources:["recharge","winning","gift"],review_mode:"manual",turnover_multiple:"0"};
  assert.ok(validate("FinanceWithdrawalBrandConfig")(brand));
  assert.ok(!validate("FinanceWithdrawalBrandWriteConfig")(brand));
  assert.ok(validate("FinanceWithdrawalBrandWriteConfig")({...brand,turnover_multiple:"1"}));
  assert.ok(validate("FinanceWithdrawalBrandWriteConfig")({...brand,allowed_sources:["recharge","winning","gift","commission"],turnover_multiple:"1"}));
});
test("wallet is a strict four-source DTO while immutable ledger snapshots retain a legacy three-source shape",()=>{
  const schemas=doc.components.schemas;
  assert.deepEqual(Object.keys(schemas.FinanceBalance.properties),["recharge","winning","gift","commission"]);
  assert.ok(schemas.FinanceWallet.required.includes("commission_points"));
  assert.equal(schemas.FinanceBalance.additionalProperties,false);
  assert.deepEqual(schemas.FinanceEntry.properties.before_snapshot,{$ref:"#/components/schemas/FinanceLedgerBalance"});
  assert.deepEqual(schemas.FinanceEntry.properties.after_snapshot,{$ref:"#/components/schemas/FinanceLedgerBalance"});
  assert.deepEqual(schemas.FinanceEntry.properties.delta_snapshot,{$ref:"#/components/schemas/FinanceLedgerDeltaBalance"});
  assert.ok(schemas.FinanceLedgerBalance.anyOf.some((schema)=>schema.$ref==="#/components/schemas/FinanceLegacyBalance"));
  assert.ok(schemas.FinanceLedgerDeltaBalance.anyOf.some((schema)=>schema.$ref==="#/components/schemas/FinanceLegacyDeltaBalance"));
  assert.equal(schemas.FinanceEntry.properties.source_allocation.maxItems,4);
});
test("admin logout uses admin authentication while unsupported payout operations remain absent",()=>{
  assert.deepEqual(doc.paths["/api/v1/admin/auth/logout"].post.security,[{adminBearer:[]},{adminCookie:[]}]);
  const create=doc.paths["/api/v1/withdrawals"].post;
  assert.ok(create.parameters.some(p=>p.name==="X-Withdrawal-Actor-Context"&&p.required));
  const body=doc.components.schemas.WithdrawalCreateRequest;
  assert.deepEqual(body.required,["points","source_allocation"]);
  assert.equal(body.additionalProperties,false);
  assert.ok(create.responses["409"].description);
  assert.match(create.description,/WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED/);
  assert.ok(!doc.paths["/api/v1/admin/commissions/pay"]);
});
test("compliance admission records cannot claim anonymous users or disabled-check reviews",()=>{
  const id="11111111-1111-4111-8111-111111111111";
  const config={age_enabled:false,minimum_age:null,region_enabled:false,allowed_countries:[],identity_enabled:true};
  const row={id,brand_id:id,policy_version:2,config,operation:"registration",action:"register",decision:"review",checks:[{check:"age",enabled:false,decision:"allow",reason_code:"CHECK_DISABLED"},{check:"region",enabled:false,decision:"allow",reason_code:"CHECK_DISABLED"},{check:"identity",enabled:true,decision:"review",reason_code:"ADAPTER_NOT_CONFIGURED"}],adapter_mode:"stub",actor_type:"anonymous",actor_id:null,member_id:null,request_id:id,audit_log_id:id,created_at:"2026-10-07T00:00:00Z"};
  const check=validate("ComplianceGateRecord");assert.ok(check(row),JSON.stringify(check.errors));
  assert.ok(!check({...row,actor_id:id}));assert.ok(!check({...row,operation:"betting",action:"bet_place"}));
  assert.ok(!check({...row,config:{...config,identity_enabled:false}}));assert.ok(!check({...row,password:"not-an-admission-field"}));
  assert.ok(check({...row,actor_type:"admin",actor_id:id,action:"operator_join"}));
  assert.ok(check({...row,actor_type:"user",actor_id:id,member_id:id,operation:"betting",action:"bet_place"}));
  assert.ok(!check({...row,actor_type:"user",actor_id:id,operation:"betting",action:"bet_place"}));
});
