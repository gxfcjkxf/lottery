import {readFileSync} from 'node:fs'
import {afterEach,describe,expect,it,vi} from 'vitest'
import * as Vue from 'vue'
import {compileScript,parse} from 'vue/compiler-sfc'
import ts from 'typescript'
import {ApiError} from './api'
import {currencies,emptyDestination,pendingCashWrites} from './cash'
const renderer=Vue.createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),setText(){},setElementText(){},patchProp(){},insert(){},remove(){},parentNode:()=>null,nextSibling:()=>null})
const apps:Array<ReturnType<typeof renderer.createApp>>=[]
const settle=async()=>{await Vue.nextTick();await new Promise(resolve=>setTimeout(resolve,0))}
async function mount() {
 const requests:Array<{resolve:(value:any)=>void;reject:(cause:unknown)=>void}>=[]
 const read=vi.fn(()=>new Promise((resolve,reject)=>requests.push({resolve,reject})))
 const descriptor=parse(readFileSync(new URL('./CashAccounts.vue',import.meta.url),'utf8')).descriptor
 const compiled=compileScript(descriptor,{id:'cash-session-lifecycle'}).content
 const modules:Record<string,any>={vue:Vue,'@lottery/shared':{ApiError,useOperationToast(){},withGlobalLoading:(fn:Function,...args:any[])=>fn(...args)},'./cash':{createCashApi:()=>({state:read,saveDestination:async()=>({})}),currencies,emptyDestination,pendingCashWrites}}
 const code=ts.transpileModule(compiled,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText.replace(/^import\s+\{([^}]+)\}\s+from\s+["']([^"']+)["'];?$/gm,(_m,bindings,path)=>`const {${bindings.replace(/\s+as\s+/g,': ')}}=modules[${JSON.stringify(path)}]??{};`).replace(/^import\s+(\w+)\s+from\s+["'][^"']+["'];?$/gm,'const $1={};').replace('export default','return')
 const component=new Function('modules',code)(modules),invalid=vi.fn(),done=vi.fn(),member=Vue.reactive({id:'member',brand_id:'brand'})
 const app=renderer.createApp({...component,render:()=>null},{apiBase:'/api/v1',member,t:(key:string)=>key,errorText:String,initialCurrency:'USDT',onAuthExpired:invalid,onDone:done,returnToWithdrawal:true});apps.push(app);app.mount({});await settle()
 return {app,state:(app._instance as any).setupState,requests,invalid,done,member}
}
afterEach(()=>{apps.splice(0).forEach(app=>app.unmount());pendingCashWrites.clear()})
const value=(marker:string)=>({brand_id:'brand',member_id:'member',actor_context:'a'.repeat(64),destinations:[],marker})
describe('payment account read ordering',()=>{
 it.each([false,true])('keeps a newer read when an older response arrives, old401=%s',async old401=>{
  const {state,requests,invalid}=await mount()
  const latest=state.load();expect(requests).toHaveLength(2)
  requests[1].resolve(value('latest'));await latest;expect(state.state.marker).toBe('latest')
  if(old401)requests[0].reject(new ApiError('',401,'AUTH_SESSION_REVOKED'));else requests[0].resolve(value('obsolete'))
  await settle();expect(state.state.marker).toBe('latest');expect(state.loading).toBe(false);expect(invalid).not.toHaveBeenCalled()
  // The late initial read must not reopen its account editor after the newer read.
  expect(state.draft).toBeNull()
 })
 it('does not navigate the new member after an old post-save reload finishes',async()=>{
  const s=await mount();s.requests[0].resolve(value('initial'));await settle()
  const oldWrite=s.state.save();await settle();expect(s.requests).toHaveLength(2)
  s.member.id='next-member';await settle();expect(s.requests).toHaveLength(3)
  s.requests[2].resolve({...value('current'),member_id:'next-member'});await settle()
  s.requests[1].resolve(value('obsolete'));await oldWrite
  expect(s.state.state.marker).toBe('current');expect(s.done).not.toHaveBeenCalled()
  const currentWrite=s.state.save();await settle()
  s.requests[3].resolve({...value('current'),member_id:'next-member'});await currentWrite
  expect(s.done).toHaveBeenCalledOnce()
 })
 it('ignores an authentication failure after the account page unmounts',async()=>{
  const {app,requests,invalid}=await mount();app.unmount();apps.splice(apps.indexOf(app),1)
  requests[0].reject(new ApiError('',401,'AUTH_SESSION_REVOKED'));await settle();expect(invalid).not.toHaveBeenCalled()
 })
})
