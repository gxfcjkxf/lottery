import { describe, expect, it, vi } from "vitest";
import { AdminApiError, type AdminAccount } from "./admin-api";
import { commissionAdjustmentPermissions, createCommissionAdjustmentsApi, validCommissionAdjustmentDelta, validCommissionAdjustmentPoints, type CommissionAdjustment, type CommissionAdjustmentTarget } from "./commission-adjustments-api";

const brand="11111111-1111-4111-8111-111111111111", actor="33333333-3333-4333-8333-333333333333", payment="44444444-4444-4444-8444-444444444444", targetId="55555555-5555-4555-8555-555555555555", earning="66666666-6666-4666-8666-666666666666", ledger="77777777-7777-4777-8777-777777777777", audit="88888888-8888-4888-8888-888888888888", adjustmentId="99999999-9999-4999-8999-999999999999", now="2026-10-07T00:00:00Z";
const account:AdminAccount={id:actor,super_admin:false,brand_ids:[brand],permissions:[],permissions_by_brand:{[brand]:["commission.view.brand","commission_adjustment.write.brand"]}};
function ok(data:unknown,status=200){return new Response(JSON.stringify({success:true,data}),{status});}
function paidTarget(overrides:Partial<CommissionAdjustmentTarget>={}):CommissionAdjustmentTarget{return{id:targetId,brand_id:brand,payment_id:payment,earning_id:earning,agent_id:actor,member_id:actor,original_points:"9007199254740993",state:"paid",ledger_entry_id:ledger,paid_at:now,adjustment_version:1,adjusted_points:"9007199254740993",last_adjustment_id:null,...overrides};}
function adjustment(overrides:Partial<CommissionAdjustment>={}):CommissionAdjustment{return{id:adjustmentId,brand_id:brand,target_id:targetId,payment_id:payment,version:2,points_before:"9007199254740993",points_after:"9007199254740988",delta_points:"-5",ledger_entry_id:ledger,audit_log_id:audit,created_by:actor,reason:"Reviewed ✅",point_policy_version:"3",created_at:now,...overrides};}
describe("commission adjustments API",()=>{
  it("applies brand and role rules, including super-admin read-only",()=>{
    expect(commissionAdjustmentPermissions(account,brand)).toEqual({view:true,write:true});
    expect(commissionAdjustmentPermissions({...account,super_admin:true,platform_permissions:["commission.view.platform"],permissions_by_brand:{[brand]:["commission.view.brand","commission_adjustment.write.brand"]}},brand)).toEqual({view:false,write:false});
    expect(commissionAdjustmentPermissions({...account,permissions:["commission.view.brand","commission_adjustment.write.brand"],permissions_by_brand:undefined},brand)).toEqual({view:false,write:false});
    expect(commissionAdjustmentPermissions({...account,permissions_by_brand:{[brand]:["commission_adjustment.write.brand"]}},brand)).toEqual({view:false,write:false});
  });
  it("uses exact paths, actor and brand headers, immutable body and exact acknowledgement",async()=>{
    const fetcher=vi.fn<typeof fetch>(async(input,init)=>{const url=String(input);if(url.includes("/targets?") )return ok({brand_id:brand,payment_id:payment,items:[paidTarget()],total_count:"1",limit:20,offset:0});if(url.endsWith("/adjustments?limit=20&offset=0"))return ok({brand_id:brand,target_id:targetId,items:[adjustment()],total_count:"1",limit:20,offset:0});return ok(adjustment({version:2,points_after:"0",delta_points:"-9007199254740993",points_before:"9007199254740993"}),201);});
    const api=createCommissionAdjustmentsApi(fetcher);await expect(api.listTargets(brand,payment)).resolves.toMatchObject({items:[{original_points:"9007199254740993"}]});await expect(api.listAdjustments(brand,targetId)).resolves.toMatchObject({items:[{delta_points:"-5"}]});
    const body=Object.freeze({version:1,points:"0",reason:"Reviewed ✅"});await api.createAdjustment(brand,targetId,payment,body,actor,"adjustment-key-01");
    const [url,init]=fetcher.mock.calls[2];expect(String(url)).toBe(`/api/v1/admin/commission-payment-targets/${targetId}/adjustments`);expect(JSON.parse(String(init?.body))).toEqual(body);expect(new Headers(init?.headers).get("X-Brand-ID")).toBe(brand);expect(new Headers(init?.headers).get("X-Commission-Payment-Actor-ID")).toBe(actor);expect(new Headers(init?.headers).get("Idempotency-Key")).toBe("adjustment-key-01");
  });
  it("rejects floating point inputs, incorrect deltas and malformed or mismatched receipts",async()=>{
    const api=createCommissionAdjustmentsApi(async()=>ok(adjustment({points_before:"1",points_after:"3",delta_points:"1"})));
    await expect(api.createAdjustment(brand,targetId,payment,{version:1,points:"1.0",reason:"x"},actor,"adjustment-key-01")).rejects.toBeInstanceOf(AdminApiError);
    await expect(api.createAdjustment(brand,targetId,payment,{version:1,points:"0",reason:"x"},actor,"adjustment-key-01")).rejects.toMatchObject({status:502,code:"INVALID_RESPONSE"});
    await expect(createCommissionAdjustmentsApi(async()=>ok({brand_id:brand,payment_id:payment,items:[paidTarget({adjusted_points:"2",adjustment_version:1})],total_count:"1",limit:20,offset:0})).listTargets(brand,payment)).rejects.toMatchObject({status:502,code:"INVALID_RESPONSE"});
  });
  it("allows Unicode reasons within UTF-8 byte bounds and rejects malformed input",async()=>{
    const api=createCommissionAdjustmentsApi(async()=>ok(adjustment(),201));
    await expect(api.createAdjustment(brand,targetId,payment,{version:1,points:"9007199254740988",reason:"Reviewed ✅"},actor,"adjustment-key-01")).resolves.toMatchObject({delta_points:"-5"});
    await expect(api.createAdjustment(brand,targetId,payment,{version:1,points:"0",reason:"✅".repeat(126)},actor,"adjustment-key-01")).rejects.toBeInstanceOf(AdminApiError);
    await expect(api.createAdjustment(brand,targetId,payment,{version:1,points:"0",reason:"\ud800"},actor,"adjustment-key-01")).rejects.toBeInstanceOf(AdminApiError);
  });
  it("caps adjusted points and signed deltas at int64 without number coercion",()=>{
    expect(validCommissionAdjustmentPoints("9223372036854775807")).toBe(true);expect(validCommissionAdjustmentPoints("9223372036854775808")).toBe(false);
    expect(validCommissionAdjustmentDelta("18446744073709551615","0")).toBe(false);expect(validCommissionAdjustmentDelta("9223372036854775808","0")).toBe(true);
    expect(validCommissionAdjustmentDelta("0","9223372036854775807")).toBe(true);expect(validCommissionAdjustmentDelta("9007199254740993","9007199254740988")).toBe(true);
  });
  it("validates the Go target and history page envelopes, string counts, and empty pages beyond total",async()=>{
    const targetPage={brand_id:brand,payment_id:payment,items:[],total_count:"1",limit:20,offset:8};
    await expect(createCommissionAdjustmentsApi(async()=>ok(targetPage)).listTargets(brand,payment,20,8)).resolves.toEqual(targetPage);
    await expect(createCommissionAdjustmentsApi(async()=>ok({...targetPage,total_count:1})).listTargets(brand,payment,20,8)).rejects.toMatchObject({status:502,code:"INVALID_RESPONSE"});
    await expect(createCommissionAdjustmentsApi(async()=>ok({...targetPage,other_id:payment})).listTargets(brand,payment,20,8)).rejects.toMatchObject({status:502,code:"INVALID_RESPONSE"});
    const historyPage={brand_id:brand,target_id:targetId,items:[],total_count:"0",limit:20,offset:100};
    await expect(createCommissionAdjustmentsApi(async()=>ok(historyPage)).listAdjustments(brand,targetId,20,100)).resolves.toEqual(historyPage);
    await expect(createCommissionAdjustmentsApi(async()=>ok({...historyPage,brand_id:brand,target_id:payment})).listAdjustments(brand,targetId,20,100)).rejects.toMatchObject({status:502,code:"INVALID_RESPONSE"});
    await expect(createCommissionAdjustmentsApi(async()=>ok(historyPage)).listAdjustments("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa".toUpperCase(),targetId)).rejects.toMatchObject({status:0,code:"INVALID_INPUT"});
  });
});
