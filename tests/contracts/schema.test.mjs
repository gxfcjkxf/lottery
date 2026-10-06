import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";
import {spawnSync} from "node:child_process";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const ajv=new Ajv2020({strict:false,allErrors:true});
addFormats(ajv);
// OpenAPI annotates integers as int64; exact financial integers use strings.
ajv.addFormat("int64", {type:"number",validate:Number.isInteger});
const root={$id:"urn:lottery:implemented-api",components:doc.components};
ajv.addSchema(root);
const validate=schema=>ajv.compile({$ref:`urn:lottery:implemented-api#/components/schemas/${schema}`});

test("every component and operation body is a compilable JSON Schema",()=>{
  for(const name of Object.keys(doc.components.schemas))validate(name);
  for(const item of Object.values(doc.paths))for(const operation of Object.values(item)){
    const schemas=[operation.requestBody?.content?.["application/json"]?.schema,...Object.values(operation.responses).map(r=>r.content?.["application/json"]?.schema)].filter(Boolean);
    for(const schema of schemas)ajv.compile({...schema,components:doc.components});
  }
});
test("actual Go DTO serialization and rule-engine outputs satisfy contracts",()=>{
  const result=spawnSync(process.env.LOTTERY_GO_BIN??"go",["run","-buildvcs=false","./cmd/contract-examples"],{cwd:new URL("../../backend/",import.meta.url),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"}});
  assert.equal(result.status,0,result.stderr||result.error?.message);
  const examples=JSON.parse(result.stdout);
  assert.ok(Object.keys(examples).length>=15);
  for(const [name,value] of Object.entries(examples)){
    const check=validate(name.replace(/Sparse$/, ""));assert.ok(check(value),`${name}: ${JSON.stringify(check.errors)}`);
  }
  assert.equal(examples.LotterySimulationResult.bet_points,"8");
  assert.equal(examples.LotterySimulationResult.prize_points,"70");
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
test("admin logout uses admin authentication and unavailable payout operations are absent",()=>{
  assert.deepEqual(doc.paths["/api/v1/admin/auth/logout"].post.security,[{adminBearer:[]},{adminCookie:[]}]);
  assert.ok(!doc.paths["/api/v1/withdrawals"]);assert.ok(!doc.paths["/api/v1/admin/commissions/pay"]);
});
