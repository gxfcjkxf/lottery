import {readFileSync} from 'node:fs'
import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest'
import * as Vue from 'vue'
import {compileScript,parse} from 'vue/compiler-sfc'
import ts from 'typescript'
import {useSessionReplacement} from '../../shared/src/session-replacement'
import {AgentError,pendingAgentTransfers} from './api'
import {agentPercentageToRatio,validAgentPercentage} from '../../admin-web/src/agent-percentage'
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',actor='aaaaaaaa-aaaa-4aaa-8aaa-000000000002'
const owned=(allowed=true)=>({account:{agent_account_id:actor,username:'agent_trial'},agent:{id:actor,brand_id:brand,policy_version:1,version:1,config:{can_create_children:allowed}},code:null,user_origin:null,brand_code:'trial'})
let latest=owned()
const request=vi.fn<(path:string,body?:unknown,key?:string)=>Promise<any>>()
const apps:Array<ReturnType<typeof renderer.createApp>>=[]
const renderer=Vue.createRenderer({createElement:()=>({}),createText:()=>({}),createComment:()=>({}),setText(){},setElementText(){},patchProp(){},insert(){},remove(){},parentNode:()=>null,nextSibling:()=>null})
const settle=async()=>{await Vue.nextTick();await new Promise(resolve=>setTimeout(resolve,0))}
async function mount(file='App.vue',props:Record<string,unknown>={}) {
 const descriptor=parse(readFileSync(new URL('./'+file,import.meta.url),'utf8')).descriptor
 const compiled=compileScript(descriptor,{id:'agent-session-lifecycle'}).content
 const modules:Record<string,any>={vue:Vue,'./api':{request,AgentError,pendingAgentTransfers},'./i18n':{i18n:{t:(key:string)=>key,locale:Vue.ref('en'),message:(key:string)=>key,errorText:String,errorMessage:String,displayError:String}},'../../admin-web/src/agent-percentage':{agentPercentageToRatio,validAgentPercentage},'@lottery/shared':{useSessionReplacement,useOperationToast(){},withGlobalLoading:(run:Function,...args:any[])=>run(...args),loadPlatformDisplaySettings:async()=>{},createApiClient:()=>({getContext:async()=>({brand:{id:brand,name:'Trial',logoText:'T'}})}),applyBrowserBranding(){},safeBrandAssetUrl:()=>null,setInlineValidationLocale(){},writeLocaleCookie(){}}}
 const code=ts.transpileModule(compiled,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText.replace(/^import\s+\{([^}]+)\}\s+from\s+["']([^"']+)["'];?$/gm,(_m,bindings,path)=>`const {${bindings.replace(/\s+as\s+/g,': ')}}=modules[${JSON.stringify(path)}]??{};`).replace(/^import\s+(\w+)\s+from\s+["'][^"']+["'];?$/gm,'const $1={};').replace('export default','return')
 const component=new Function('modules',code)(modules)
 const app=renderer.createApp({...component,render:()=>null},props);apps.push(app);app.mount({});await settle()
 return {app,state:(app._instance as any).setupState}
}
beforeEach(()=>{latest=owned();request.mockReset();request.mockImplementation(async path=>path==='/me'?latest:path==='/wallet'?{available_points:'100',commission_points:'0'}:{});vi.stubGlobal('window',new EventTarget());vi.stubGlobal('location',{hash:''});vi.stubGlobal('document',{documentElement:{lang:''},title:''})})
afterEach(()=>{apps.splice(0).forEach(app=>app.unmount());pendingAgentTransfers.clear();vi.unstubAllGlobals()})
describe('agent session lifecycle',()=>{
 it.each([401,503])('shows a usable initial state for status %s',async status=>{
  request.mockRejectedValueOnce(new AgentError('',status,status===401?'AUTH_SESSION_REVOKED':'SERVICE_UNAVAILABLE'))
  const {state}=await mount();expect(state.ready).toBe(true);expect(state.me).toBeNull()
  if(status===401)expect(state.error).toBeNull();else expect(state.error.status).toBe(503)
 })
 it('applies capabilities from session checks and blocks a revoked child editor',async()=>{
  const {state}=await mount();expect(state.ready).toBe(true);expect(state.me.agent.config.can_create_children).toBe(true)
  latest=owned(false);window.dispatchEvent(new Event('focus'));await settle()
  expect(state.me.agent.config.can_create_children).toBe(false);state.startEditor('children');expect(state.editor).toBeNull();expect(state.error.status).toBe(403)
 })
 it.each([false,true])('ignores an old identity response after clear, error=%s',async error=>{
  const {state}=await mount();let finish!:(value:unknown)=>void,fail!:(cause:unknown)=>void
  request.mockImplementationOnce(()=>new Promise((resolve,reject)=>{finish=resolve;fail=reject}))
  const read=state.refresh();state.clearPrivate();if(error)fail(new AgentError('',401,'AUTH_SESSION_REVOKED'));else finish(owned())
  await read;expect(state.me).toBeNull();expect(state.ready).toBe(true);expect(state.busy).toBe(false)
 })
 it('ignores a child session event captured before a capability refresh',async()=>{
  const {state}=await mount();const oldHandler=state.sessionInvalidHandler()
  latest=owned(false);await state.refresh();oldHandler();expect(state.me).not.toBeNull()
  state.sessionInvalidHandler()();expect(state.me).toBeNull()
 })
 it('keeps an uncertain child creation key while its capability changes',async()=>{
  const {state}=await mount();state.startEditor('children');state.draft={username:'trial_child',password:'test-password',phone:'',ratio:'10',allowChildren:false}
  const writes:any[]=[]
  request.mockImplementation(async(path,body,key)=>{if(path==='/children'){writes.push({body,key});if(writes.length===1)throw new AgentError('',503,'SERVICE_UNAVAILABLE');return {}}return latest})
  await state.save();expect(state.pending).not.toBeNull()
  latest=owned(false);await state.refresh();await state.save();expect(writes).toHaveLength(1);expect(state.pending).not.toBeNull()
  latest=owned(true);await state.refresh();await state.save();expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(state.pending).toBeNull()
 })
 it('reopens an uncertain transfer with the same target, body and key',async()=>{
  const owner=brand+':'+actor,member={id:actor,username:'original_member'}
  const writes:any[]=[]
  request.mockImplementation(async(path,body,key)=>{
   if(path==='/wallet')return {available_points:'100',commission_points:'0'}
   writes.push({body,key});if(writes.length===1)throw new AgentError('',503,'SERVICE_UNAVAILABLE');return {}
  })
  const first=await mount('TransferDialog.vue',{ownerKey:owner,member});first.state.amount='25';await first.state.transfer();expect(pendingAgentTransfers.has(owner)).toBe(true)
  first.app.unmount();apps.splice(apps.indexOf(first.app),1)
  const second=await mount('TransferDialog.vue',{ownerKey:owner,member:{id:brand,username:'different_member'}})
  await second.state.transfer();expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(writes[0].body.member_id).toBe(actor);expect(pendingAgentTransfers.has(owner)).toBe(false)
 })
 it('does not report an old transfer wallet 401 after unmount',async()=>{
  let fail!:(cause:unknown)=>void
  request.mockImplementation(()=>new Promise((_resolve,reject)=>{fail=reject}))
  const invalid=vi.fn(),{app}=await mount('TransferDialog.vue',{ownerKey:brand+':'+actor,member:{id:actor,username:'member_trial'},onSessionInvalid:invalid})
  app.unmount();apps.splice(apps.indexOf(app),1);fail(new AgentError('',401,'AUTH_SESSION_REVOKED'));await settle();expect(invalid).not.toHaveBeenCalled()
 })
})
