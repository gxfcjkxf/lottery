import { afterEach, describe, expect, it } from "vitest";
import { classifyCommissionAdjustmentFailure, clearAllPendingCommissionAdjustments, createCommissionAdjustmentIntent, getPendingCommissionAdjustmentIntent, isCommissionAdjustmentConflict, markCommissionAdjustmentConflict, setPendingCommissionAdjustmentIntent } from "./commission-adjustments-state";

const account="11111111-1111-4111-8111-111111111111",brand="22222222-2222-4222-8222-222222222222",payment="33333333-3333-4333-8333-333333333333",target="44444444-4444-4444-8444-444444444444",actor="55555555-5555-4555-8555-555555555555";
const scope={accountId:account,brandId:brand,paymentId:payment,targetId:target};
afterEach(()=>clearAllPendingCommissionAdjustments());
describe("commission adjustment pending state",()=>{
  it("freezes actor and body and restores the same intent by account, brand, payment and target",()=>{
    const body={version:1,points:"0",reason:"Reviewed"};const intent=createCommissionAdjustmentIntent(scope,actor,"45",body,"adjustment-key-01")!;body.reason="mutated";
    setPendingCommissionAdjustmentIntent(scope,intent);expect(getPendingCommissionAdjustmentIntent(scope)).toEqual({...scope,actorId:actor,pointsBefore:"45",body:{version:1,points:"0",reason:"Reviewed"},key:"adjustment-key-01"});
    expect(getPendingCommissionAdjustmentIntent({...scope,accountId:"66666666-6666-4666-8666-666666666666"})).toBeNull();
    expect(getPendingCommissionAdjustmentIntent({...scope,targetId:"77777777-7777-4777-8777-777777777777"})).toBeNull();
  });
  it("keeps the pending intent after unmount semantics and requires explicit conflict state",()=>{
    const intent=createCommissionAdjustmentIntent(scope,actor,"10",{version:4,points:"12",reason:"Retry frozen"},"adjustment-key-02")!;setPendingCommissionAdjustmentIntent(scope,intent);
    expect(getPendingCommissionAdjustmentIntent(scope)?.body.version).toBe(4);markCommissionAdjustmentConflict(intent);expect(isCommissionAdjustmentConflict(intent)).toBe(true);expect(getPendingCommissionAdjustmentIntent(scope)?.key).toBe("adjustment-key-02");
    const second=createCommissionAdjustmentIntent(scope,actor,"12",{version:5,points:"13",reason:"New operation"},"adjustment-key-03")!;
    expect(setPendingCommissionAdjustmentIntent(scope,second)).toBe(false);expect(getPendingCommissionAdjustmentIntent(scope)?.key).toBe(intent.key);
  });
  it("classifies uncertain transport separately from 409 and definitive errors",()=>{
    expect(classifyCommissionAdjustmentFailure(0)).toBe("unknown");expect(classifyCommissionAdjustmentFailure(502)).toBe("unknown");expect(classifyCommissionAdjustmentFailure(409)).toBe("conflict");expect(classifyCommissionAdjustmentFailure(422)).toBe("definitive");
  });
  it("rejects noncanonical uppercase SQL UUIDs and deltas outside signed int64",()=>{
    expect(createCommissionAdjustmentIntent({...scope,brandId:"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa".toUpperCase()},actor,"10",{version:1,points:"12",reason:"R"},"adjustment-key-03")).toBeNull();
    expect(createCommissionAdjustmentIntent(scope,actor,"1",{version:1,points:"9223372036854775808",reason:"R"},"adjustment-key-04")).toBeNull();
  });
});
