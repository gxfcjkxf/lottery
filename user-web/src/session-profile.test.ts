import {readFileSync} from 'node:fs'
import {describe,it,expect} from 'vitest'
import {ref} from 'vue'
import ts from 'typescript'
import {ApiError} from '../../shared/src/api'
function profile(name:string){return {user:{id:'user',username:name,status:'active'},member:{id:'member',brand_id:'brand',display_name:name,status:'normal'}}}
function subject(){
 const source=readFileSync(new URL('./Page.vue',import.meta.url),'utf8')
 const load=source.slice(source.indexOf('async function loadProfile()'),source.indexOf('function expireRechargeProfile()'))
 const check=source.slice(source.indexOf('const sessionReplaced = useSessionReplacement'),source.indexOf('async function saveProfile()'))
 const requests:Array<{resolve:(value:unknown)=>void;reject:(cause:unknown)=>void}>=[]
 const authProfile=ref<any>(profile('initial')),profileLoading=ref(false)
 const api={me:()=>new Promise((resolve,reject)=>requests.push({resolve,reject}))}
 const code=ts.transpileModule(`let authGeneration=0,profileReadGeneration=0,pageGeneration=0;const profileSaving={value:false},authBusy={value:false},profileError={value:''},profileNotice={value:''},profileUsername={value:''},profilePhone={value:''},profileContacts={value:{}},contactKeys=[],brandContextId={value:'brand'},route={path:'/account',query:{}},router={replace:async()=>{}},ui=(key)=>key,publicErrorMessage=String;let checkSession;function useSessionReplacement(active,check,clear){checkSession=check;return {value:false}}function expireRechargeProfile(){authGeneration++;profileReadGeneration++;authProfile.value=null;profileLoading.value=false}${load}\n${check}\nreturn {loadProfile,check:()=>checkSession()};`,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.None}}).outputText
 const functions=new Function('authApi','authProfile','profileLoading','ApiError',code)(api,authProfile,profileLoading,ApiError)
 return {...functions,requests,authProfile,profileLoading}
}
describe('foreground and background profile read ordering',()=>{
 it.each([false,true])('does not let an older profile response replace a newer check, old401=%s',async old401=>{
  const s=subject(),old=s.loadProfile(),latest=s.check()
  expect(s.requests).toHaveLength(2)
  s.requests[1].resolve(profile('latest'));await latest
  if(old401)s.requests[0].reject(new ApiError('',401,'AUTH_SESSION_REVOKED'));else s.requests[0].resolve(profile('obsolete'))
  await old
  expect(s.authProfile.value.member.display_name).toBe('latest')
  expect(s.profileLoading.value).toBe(false)
 })
})
