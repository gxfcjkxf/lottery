import { describe, it, expect, vi, afterEach } from "vitest";
import { exchangeResult, minor, decimal, isCashConfig, isCashState, currencies, emptyDestination, createCashApi } from "./cash";
import { cashCatalog } from "./cash-catalog";
describe("currency money rules", () => {
  it("floors destination amounts without float drift or roundtrip gain", () => {
    expect(exchangeResult("1", "7", false, "USDT")).toBe("0.142857");
    expect(exchangeResult("1", "7", false, "CNY")).toBe("0.14");
    expect(exchangeResult("123.456789", "7", true, "USDT")).toBe("864.19");
    expect(exchangeResult("0.142857", "7", true, "USDT")).toBe("0.99");
    expect(decimal(minor("0.1") + minor("0.2"))).toBe("0.3");
  });
  it("requires the complete currency set and translations", () => {
    expect(isCashConfig({ version: 0, rules: [] })).toBe(false);
    expect(Object.keys(cashCatalog.en).sort()).toEqual(
      Object.keys(cashCatalog["zh-CN"]).sort(),
    );
  });
});

describe('personal payment accounts', () => {
  afterEach(() => vi.unstubAllGlobals());
  const id = 'aaaaaaaa-aaaa-4aaa-8aaa-000000000001';
  const binding = {...emptyDestination('CNY'), id, holder:'Recipient',institution:'Bank',number:'123456',version:1};
  it('rejects two bindings for one currency while accepting different currencies', () => {
    const config = {version:1,rules:currencies.map(currency => ({currency,inherit:false,rate:'7',deposit_min:'1',deposit_max:'10000',withdrawal_min:'1',withdrawal_max:'10000',fee:'0',mode:'count',threshold:'1',enabled:true}))};
    const state = {auto_usdt:false,actor_context:'a'.repeat(64),brand_id:id,member_id:id,platform:config,brand:config,balances:currencies.map(currency=>({currency,deposited:'0',withdrawable:'0',reserved:'0'})),destinations:[binding]};
    expect(isCashState(state)).toBe(true);
    expect(isCashState({...state,destinations:[binding,{...binding,id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000002'}]})).toBe(false);
    expect(isCashState({...state,destinations:[binding,{...binding,currency:'USD'}]})).toBe(true);
  });
  it('pins deletion to the original binding, version, actor and idempotency key', async () => {
    const fetcher=vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({success:true,data:{...binding,version:2,enabled:false}}),{status:200}));
    vi.stubGlobal('fetch',fetcher);
    const api=createCashApi('/api/v1',undefined,'a'.repeat(64));
    await expect(api.deleteDestination({id,version:1},'delete-key')).resolves.toMatchObject({id,version:2,enabled:false});
    expect(fetcher).toHaveBeenCalledWith('/api/v1/cash/destinations/delete',expect.objectContaining({method:'POST',body:JSON.stringify({id,version:1}),headers:expect.objectContaining({'Idempotency-Key':'delete-key','X-Cash-Actor-Context':'a'.repeat(64)})}));
  });
  it('rejects a save response for a different currency or version', async () => {
    const fetcher=vi.fn<typeof fetch>().mockImplementation(async()=>new Response(JSON.stringify({success:true,data:{...binding,currency:'USD',version:2}}),{status:200}));
    vi.stubGlobal('fetch',fetcher);
    await expect(createCashApi('/api/v1').saveDestination(binding,'save-key')).rejects.toMatchObject({code:'INVALID_RESPONSE'});
  });
});
