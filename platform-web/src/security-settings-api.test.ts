import { describe, expect, it } from 'vitest';
import { createSecuritySettingsApi, securityIntents, type SecurityIntent } from './security-settings-api';
const brand='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const auth={version:3,inherit_platform:false,captcha_enabled:true,telegram_enabled:false,telegram_client_id:'',audit_log_id:brand};
const intent:SecurityIntent={kind:'auth',body:JSON.stringify({version:2,inherit_platform:false,captcha_enabled:true,telegram_enabled:false,telegram_client_id:'',reason:''}),key:'original-key',phase:'unknown'};
describe('platform security settings',()=>{
 it('keeps the original body and key for an uncertain write and validates its receipt',async()=>{
  const calls:RequestInit[]=[];
  const api=createSecuritySettingsApi(async(url,options)=>{expect(url).toBe('/api/v1/platform/auth-settings');calls.push(options!);return Response.json({success:true,data:auth});});
  await api.write(brand,intent);await api.write(brand,intent);
  expect(calls[0].body).toBe(intent.body);expect(calls[1].body).toBe(intent.body);
  expect(new Headers(calls[1].headers).get('Idempotency-Key')).toBe(intent.key);
  expect(new Headers(calls[0].headers).get('X-Brand-ID')).toBe(brand);
 });
 it('does not accept mismatched values or missing audit receipts as success',async()=>{
  for(const data of [{...auth,captcha_enabled:false},{...auth,audit_log_id:''},{...auth,version:8}]){
   const api=createSecuritySettingsApi(async()=>Response.json({success:true,data}));
   await expect(api.write(brand,intent)).rejects.toMatchObject({code:'INVALID_RESPONSE',status:0});
  }
 });
 it('retains a detached immutable intent by account/brand and clears it on logout',()=>{
  const scope='operator:'+brand;securityIntents.set(scope,intent);
  expect(Object.isFrozen(securityIntents.get(scope))).toBe(true);
  const before=securityIntents.epoch();securityIntents.clearAll();expect(securityIntents.get(scope)).toBeNull();expect(securityIntents.epoch()).toBe(before+1);
 });
});

it('reads platform defaults without a brand header',async()=>{
 const api=createSecuritySettingsApi(async(_url,options)=>{expect(new Headers(options?.headers).has('X-Brand-ID')).toBe(false);return Response.json({success:true,data:auth})});
 await api.read('auth','');
});
it('accepts the effective platform values when a brand restores inheritance',async()=>{
 const inherited={...auth,inherit_platform:true,captcha_enabled:false};
 const api=createSecuritySettingsApi(async()=>Response.json({success:true,data:inherited}));
 await api.write(brand,{...intent,body:JSON.stringify({...JSON.parse(intent.body),inherit_platform:true})});
});
