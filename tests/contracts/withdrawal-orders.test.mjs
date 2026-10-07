import {test} from "node:test";
import assert from "node:assert/strict";
import {readFileSync} from "node:fs";

const doc=JSON.parse(readFileSync(new URL("../../docs/openapi.json",import.meta.url),"utf8"));
const schemas=doc.components.schemas;
const paths=doc.paths;
const expectedOrderFields=[
  "id","brand_id","member_id","account_id","points","state","version","source_allocation",
  "reserve_entry_id","release_entry_id","paid_entry_id","cycle_from_at","cycle_from_version",
  "reserve_version","created_at","updated_at","reviewed_at","completed_at","decision_reason","audit_log_id",
];

test("withdrawal DTOs are closed and expose only the agreed fields",()=>{
  assert.deepEqual(Object.keys(schemas.WithdrawalOrder.properties),expectedOrderFields);
  assert.equal(schemas.WithdrawalOrder.additionalProperties,false);
  assert.deepEqual(Object.keys(schemas.WithdrawalOrderPage.properties),["brand_id","items","limit","offset","has_more"]);
  assert.deepEqual(Object.keys(schemas.WithdrawalHistory.properties),["brand_id","order_id","items"]);
  assert.deepEqual(Object.keys(schemas.WithdrawalTransition.properties),["id","version","from_state","to_state","reason","actor_type","created_at","audit_log_id"]);
  assert.deepEqual(Object.keys(schemas.WithdrawalAvailability.properties),[
    "brand_id","member_id","policy_enabled","eligibility_configured","can_apply","reason_code",
    "min_points","max_points","allowed_sources","real_payments","actor_context",
  ]);
  assert.equal(schemas.WithdrawalAvailability.properties.real_payments.const,false);
  assert.equal(schemas.WithdrawalAvailability.properties.actor_context.pattern,"^[0-9a-f]{64}$");
  assert.deepEqual(schemas.WithdrawalOrder.properties.cycle_from_version,{ $ref:"#/components/schemas/NonnegativeInt64String" });
  assert.deepEqual(schemas.WithdrawalOrder.properties.reserve_version,{ $ref:"#/components/schemas/PositiveInt64String" });
  assert.deepEqual(schemas.WithdrawalAvailability.properties.min_points,{ $ref:"#/components/schemas/PositiveInt64String" });
  assert.deepEqual(schemas.WithdrawalAvailability.properties.max_points.anyOf[0],{ $ref:"#/components/schemas/PositiveInt64String" });
  assert.equal(schemas.WithdrawalAvailability.properties.allowed_sources.minItems,1);
  assert.equal(schemas.WithdrawalAvailability.properties.allowed_sources.maxItems,3);
  assert.equal(schemas.WithdrawalAvailability.properties.allowed_sources.uniqueItems,true);
  assert.equal(schemas.WithdrawalHistory.properties.items.maxItems,3);
  assert.equal(schemas.WithdrawalOrderPage.properties.limit.minimum,1);
  assert.equal(schemas.WithdrawalOrderPage.properties.limit.maximum,100);
  assert.equal(schemas.WithdrawalOrderPage.properties.offset.minimum,0);
  assert.equal(schemas.WithdrawalOrderPage.properties.offset.maximum,1000000);
  assert.equal(schemas.WithdrawalOrderPage.properties.items.maxItems,100);
  assert.ok(schemas.WithdrawalOrderPage.allOf.some(rule=>rule.if.properties.has_more.const===true&&rule.then.properties.items.minItems===1));
  assert.equal(schemas.WithdrawalOrderPage.additionalProperties,false);
  assert.ok(!JSON.stringify(schemas.WithdrawalOrder).match(/policy_snapshot|eligibility_evidence|actor_id|client_key/));
});

test("user routes bind create to availability context and keep request bodies minimal",()=>{
  const availability=paths["/api/v1/withdrawal-availability"].get;
  const create=paths["/api/v1/withdrawals"].post;
  const header=create.parameters.find(p=>p.name==="X-Withdrawal-Actor-Context");
  assert.equal(header.required,true);
  assert.match(header.description,/cached idempotent replays/i);
  assert.equal(create.responses["201"].content["application/json"].schema.properties.success.const,true);
  assert.deepEqual(schemas.WithdrawalCreateRequest.required,["points","source_allocation"]);
  assert.equal(schemas.WithdrawalCreateRequest.additionalProperties,false);
  assert.deepEqual(Object.keys(schemas.WithdrawalCreateRequest.properties),["points","source_allocation"]);
  assert.match(availability.description,/actual global user and brand member/);
  assert.match(create.description,/WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED/);
  assert.match(create.description,/explicit nil dependency remains closed/);
  assert.match(create.description,/saved automatic review advances to processing v2/);
  assert.match(create.description,/Neither marks paid nor executes an external payment/);
  assert.deepEqual(paths["/api/v1/b/{brandCode}/withdrawals"].post.operationId,"createWithdrawalOrderByBrand");
});

test("readonly turnover preview is member-scoped, exact, closed and never a submission ticket",()=>{
  const preview=paths["/api/v1/withdrawal-qualification"].get;
  assert.deepEqual(preview.security,[{userBearer:[]}]);
  assert.ok(!preview.requestBody);
  assert.ok(!(preview.parameters??[]).some(p=>p.in==="query"));
  assert.equal(paths["/api/v1/b/{brandCode}/withdrawal-qualification"].get.operationId,"getWithdrawalQualificationByBrand");
  assert.deepEqual(Object.keys(schemas.WithdrawalQualification.properties),["brand_id","member_id","account_id","base_points","valid_points","valid_order_count","credit_numerator","credit_denominator","meets_turnover","cycle_from_at","cycle_from_version","cutoff_at","cutoff_version"]);
  assert.equal(schemas.WithdrawalQualification.additionalProperties,false);
  assert.equal(schemas.WithdrawalQualification.properties.credit_denominator.pattern,"^[1-9][0-9]*$");
  assert.match(preview.description,/Primary-only/);
  assert.match(preview.description,/POST independently rechecks/);
  assert.match(preview.description,/WITHDRAWAL_TURNOVER_BUSY/);
  assert.match(preview.description,/WITHDRAWAL_TURNOVER_EVIDENCE_INVALID/);
  assert.match(preview.description,/no points, orders, successful cycles or idempotency receipts are changed/i);
  assert.ok(!JSON.stringify(schemas.WithdrawalQualification).match(/rule_snapshot|rule_revision|actor_context|order_snapshot_digest|eligibility_evidence/));
});

test("admin read and write scopes, statuses, and action bodies are exact",()=>{
  const list=paths["/api/v1/admin/withdrawals"].get;
  assert.deepEqual(list["x-permissions"],["withdrawal.view.brand","withdrawal.view.platform"]);
  assert.deepEqual(list.parameters.filter(p=>p.in==="query").map(p=>p.name),["limit","offset","state","member_id"]);
  assert.equal(list.parameters.find(p=>p.name==="limit").schema.default,20);
  assert.equal(list.parameters.find(p=>p.name==="offset").schema.default,0);
  for(const action of ["approve","reject","cancel","fail","mark-paid"]){
    const op=paths[`/api/v1/admin/withdrawals/{id}/${action}`].post;
    const permission=action==="mark-paid"?"withdrawal.mark_paid.brand":`withdrawal.${action}.brand`;
    assert.deepEqual(op["x-permissions"],[permission]);
    assert.ok(op.responses["200"]);
    assert.deepEqual(schemas.WithdrawalActionRequest.required,["version","reason"]);
    assert.equal(schemas.WithdrawalActionRequest.additionalProperties,false);
    assert.match(op.description,/SUPER_ADMIN status never authorize writes/);
  }
  assert.match(paths["/api/v1/admin/withdrawals/{id}"].get.description,/full decision_reason/);
  assert.match(paths["/api/v1/withdrawals/{id}"].get.description,/rejection, failure, or cancellation/);
  assert.match(paths["/api/v1/admin/withdrawals/{id}/history"].get.description,/never actor IDs/);
});

test("strict list filters and no-query reads are documented without write shortcuts",()=>{
  const userList=paths["/api/v1/withdrawals"].get;
  const adminList=paths["/api/v1/admin/withdrawals"].get;
  assert.deepEqual(userList.parameters.filter(p=>p.in==="query").map(p=>p.name),["limit","offset","state"]);
  assert.match(userList.description,/unknown, duplicate, or empty query parameters are rejected/i);
  for(const path of ["/api/v1/withdrawal-availability","/api/v1/withdrawals/{id}","/api/v1/withdrawals/{id}/history","/api/v1/admin/withdrawals/{id}","/api/v1/admin/withdrawals/{id}/history"]){
    assert.ok(!Object.values(paths[path].get.parameters??[]).some(p=>p.in==="query"),`${path} has no query parameters`);
  }
  assert.ok(!paths["/api/v1/withdrawals/{id}"].patch);
  assert.ok(!paths["/api/v1/withdrawals/{id}"].delete);
  assert.ok(!adminList.parameters.some(p=>p.name==="actor_context"));
});
