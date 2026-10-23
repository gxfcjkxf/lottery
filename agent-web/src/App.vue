<script setup lang="ts">
import RegistrationLink from "@lottery/shared/RegistrationLink.vue"
import { registrationShareLink } from "@lottery/shared"
import AccountCreateFields from "@lottery/shared/AccountCreateFields.vue"
import MemberDetailsFields from "@lottery/shared/MemberDetailsFields.vue"
import AgentRatioField from "@lottery/shared/AgentRatioField.vue"
import WriteFormActions from "@lottery/shared/WriteFormActions.vue"
import AccountProfileDialog from "@lottery/shared/AccountProfileDialog.vue"
import ChangePasswordDialog from "@lottery/shared/ChangePasswordDialog.vue"
import {computed,onMounted,onBeforeUnmount,ref,watch} from 'vue'
import LoginEntry from '@lottery/shared/LoginEntry.vue'
import LoginCredentials from '@lottery/shared/LoginCredentials.vue'
import LanguageSwitcher from '@lottery/shared/LanguageSwitcher.vue'
import LoadingOverlay from '@lottery/shared/LoadingOverlay.vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import ManagementShell from '@lottery/shared/ManagementShell.vue'
import AccountMenu from '@lottery/shared/AccountMenu.vue'
import {applyBrowserBranding,safeBrandAssetUrl,vWriteGuard,vInlineValidation,setInlineValidationLocale,loadPlatformDisplaySettings,nativeLocaleName,writeLocaleCookie,withGlobalLoading,createApiClient,type PlatformContext} from '@lottery/shared'
import {agentPercentageToRatio,validAgentPercentage} from '../../admin-web/src/agent-percentage'
import MembersPage from './pages/MembersPage.vue'
import ChildrenPage from './pages/ChildrenPage.vue'
import MemberBetsPage from './pages/MemberBetsPage.vue'
import CommissionPage from './pages/CommissionPage.vue'
import {AgentError,request,brandCode} from './api'
import {i18n} from './i18n'
import type {Me,Member,MemberScope} from './types'
const {t,locale}=i18n
const brandContext=ref<PlatformContext|null>(null)
const me=ref<Me|null>(null),ready=ref(false),busy=ref(false),error=ref<unknown>(null),notice=ref(''),username=ref(''),password=ref(''),revision=ref(0)
const hash=ref(location.hash),page=computed(()=>hash.value.replace(/^#\/?/,'').split('?')[0]||'members'),query=computed(()=>new URLSearchParams(hash.value.split('?')[1]??''))
const navigation=computed(()=>[{id:'members',label:t('membersPage'),icon:'users' as const},{id:'children',label:t('childrenPage'),icon:'network' as const},{id:'bets',label:t('betsPage'),icon:'ticket' as const},{id:'commission',label:t('commissionPage'),icon:'wallet' as const}])
const pageTitle=computed(()=>navigation.value.find(item=>item.id===page.value)?.label??t('notFound'))
const editor=ref<'members'|'children'|null>(null),draft=ref({username:'',password:'',display_name:'',phone:'',ratio:'',allowChildren:false}),pending=ref<{path:string;body:unknown;key:string}|null>(null)
const passwordOpen=ref(false),profileOpen=ref(false),passwordBusy=ref(false),passwordError=ref<unknown>(null),passwordValidation=ref(false),currentPassword=ref(''),newPassword=ref(''),confirmPassword=ref(''),passwordPending=ref<{body:{current_password:string;new_password:string};key:string}|null>(null)
const formValid=computed(()=>/^[a-z][a-z0-9_]{2,31}$/.test(draft.value.username)&&draft.value.password.length>=6&&draft.value.password.length<=30&&(editor.value==='members'||validAgentPercentage(draft.value.ratio)))
const registrationLink=computed(()=>{
 if(!me.value?.code)return null
 try{return registrationShareLink(import.meta.env.VITE_USER_APP_ORIGIN,'agent',me.value.code)}catch{return null}
})
const errorText=computed(()=>error.value?i18n.errorText(error.value):'')
let generation=0
function navigate(id:string){location.hash='/'+id}
function hashChanged(){hash.value=location.hash;if(me.value){error.value=null;notice.value=''}}
function clearPrivate(){generation++;me.value=null;pending.value=null;editor.value=null;draft.value.password='';profileOpen.value=false;passwordOpen.value=false;currentPassword.value='';newPassword.value='';confirmPassword.value='';passwordPending.value=null;passwordError.value=null;passwordValidation.value=false;navigate('members')}
function fail(cause:unknown){error.value=cause;if(cause instanceof AgentError&&cause.status===401)clearPrivate()}
async function refresh(){const ticket=++generation;try{const own=await request<Me>('/me');if(ticket===generation)me.value=own}catch(e){if(ticket===generation){fail(e);if(!ready.value)error.value=null}}finally{ready.value=true}}
async function login(){if(busy.value)return;busy.value=true;error.value=null;notice.value='';try{await request('/login',{username:username.value.trim(),password:password.value});password.value='';await refresh()}catch(e){fail(e)}finally{busy.value=false;ready.value=true}}
async function logout(){if(busy.value)return;busy.value=true;try{await request('/logout',{});clearPrivate();notice.value=''}catch(e){fail(e)}finally{busy.value=false}}
function showPassword(){passwordError.value=null;passwordValidation.value=false;currentPassword.value='';newPassword.value='';confirmPassword.value='';passwordOpen.value=true}
async function changePassword(){if(passwordBusy.value||!me.value)return;passwordError.value=null;passwordValidation.value=false
 if(!passwordPending.value){const length=new TextEncoder().encode(newPassword.value).length;if(!currentPassword.value||length<6||length>30||newPassword.value!==confirmPassword.value){passwordValidation.value=true;return}passwordPending.value={body:{current_password:currentPassword.value,new_password:newPassword.value},key:crypto.randomUUID()}}
 passwordBusy.value=true;try{await request('/password',passwordPending.value.body,passwordPending.value.key);clearPrivate();notice.value=t('passwordChanged')}catch(e){passwordError.value=e;if(e instanceof AgentError&&e.status>=400&&e.status<500)passwordPending.value=null;if(e instanceof AgentError&&e.status===401)fail(e)}finally{passwordBusy.value=false}}
function startEditor(kind:'members'|'children'){editor.value=kind;draft.value={username:'',password:'',display_name:'',phone:'',ratio:'',allowChildren:false};error.value=null;notice.value=''}
async function save(){if(busy.value||!me.value||!editor.value)return;if(!pending.value){if(!formValid.value)return;const body=editor.value==='members'?{username:draft.value.username,password:draft.value.password,display_name:draft.value.display_name,phone:draft.value.phone}:{username:draft.value.username,password:draft.value.password,policy_version:me.value.agent.policy_version,parent_id:me.value.agent.id,parent_version:me.value.agent.version,config:{ratio:agentPercentageToRatio(draft.value.ratio),mode:null,status:'active',can_create_children:draft.value.allowChildren}};pending.value={path:'/'+editor.value,body,key:crypto.randomUUID()}}
 busy.value=true;error.value=null;try{await request(pending.value.path,pending.value.body,pending.value.key);pending.value=null;editor.value=null;draft.value.password='';notice.value=t('saved');revision.value++;await refresh()}catch(e){if(e instanceof AgentError&&e.status>=400&&e.status<500)pending.value=null;fail(e)}finally{busy.value=false}}
function viewBets(member:Member,scope:MemberScope){location.hash=`/bets?member=${encodeURIComponent(member.id)}&scope=${scope}`}
function sessionInvalid(){fail(new AgentError('',401,'AUTH_SESSION_REVOKED'))}
watch([brandContext,locale],()=>{const brand=brandContext.value?.brand;if(brand)applyBrowserBranding(t('title'),brand.logoText,safeBrandAssetUrl(brand.faviconUrl),brand.primary)})
watch(locale,value=>{document.documentElement.lang=value;document.title=t('title');writeLocaleCookie(value);setInlineValidationLocale(value)},{immediate:true})
onMounted(async()=>{window.addEventListener('hashchange',hashChanged);try{await loadPlatformDisplaySettings();brandContext.value=await createApiClient({brandCode:brandCode??undefined}).getContext();await refresh()}catch(e){fail(e);ready.value=true}})
onBeforeUnmount(()=>{generation++;window.removeEventListener('hashchange',hashChanged)})
</script>
<template>
 <LoadingOverlay :active="!ready" :label="t('loading')"/>
 <LoginEntry v-if="!me" :ready="ready&&Boolean(brandContext)" :brand-name="brandContext?.brand.name??''" :logo-text="brandContext?.brand.logoText" :logo-url="brandContext?.brand.logoUrl" :subtitle="t('title')" :title="t('login')" title-id="agent-login-title" :locale="locale" :language-label="t('language')" :languages="(['en','zh-CN'] as const).map(code=>({code,label:nativeLocaleName(code)}))" @update:locale="i18n.setLocale"><template #status><p v-if="ready&&!brandContext&&error" class="message error" role="alert">{{errorText}}</p></template><form v-write-guard="busy" class="login-entry__form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(login)"><LoginCredentials v-model:identifier="username" v-model:password="password" :identifier-label="t('account')" :password-label="t('password')" :password-minlength="6" :password-maxlength="30"/><p v-if="error" role="alert" class="form-error">{{errorText}}</p><p v-if="notice" role="status">{{notice}}</p><button class="button button-primary" :disabled="busy">{{t(busy?'loading':'login')}}</button></form></LoginEntry>
 <ManagementShell v-else-if="brandContext" :title="t('title')" :brand-name="brandContext.brand.name" :logo-text="brandContext.brand.logoText" :logo-url="brandContext.brand.logoUrl" :page-title="pageTitle" :navigation-label="t('navigation')" :items="navigation" :model-value="page" @update:model-value="navigate">
  <template #actions><LanguageSwitcher :model-value="locale" :label="t('language')" :options="(['en','zh-CN'] as const).map(code=>({code,label:nativeLocaleName(code)}))" @update:model-value="i18n.setLocale"/><AccountMenu :username="me.account.username"><button type="button" role="menuitem" @click="profileOpen=true">{{t('profile')}}</button><button type="button" role="menuitem" @click="showPassword">{{t('changePassword')}}</button><button v-write-guard="busy" type="button" role="menuitem" class="popover-logout" :disabled="busy" @click="withGlobalLoading(logout)">{{t('logout')}}</button></AccountMenu></template>
  <template #heading-actions><button v-if="page==='members'||(page==='children'&&me.agent.config.can_create_children)" class="button button-primary" @click="startEditor(page as 'members'|'children')">{{t(page==='members'?'addMember':'addAgent')}}</button></template>
  <p v-if="error" class="message error" role="alert">{{errorText}}</p><p v-if="notice" role="status">{{notice}}</p>
  <MembersPage v-if="page==='members'" :key="'members'+revision" @bets="viewBets" @session-invalid="sessionInvalid"/>
  <ChildrenPage v-else-if="page==='children'" :key="'children'+revision" @session-invalid="sessionInvalid"/>
  <MemberBetsPage v-else-if="page==='bets'" :member-id="query.get('member')" :scope="(query.get('scope')??'direct') as MemberScope" @select="viewBets" @clear="navigate('bets')" @session-invalid="sessionInvalid"/>
  <CommissionPage v-else-if="page==='commission'" @session-invalid="sessionInvalid"/>
  <p v-else role="alert">{{t('notFound')}}</p>
  <AccountProfileDialog v-if="profileOpen" :title="t('profile')" :close-label="t('close')" :fields="[{label:t('account'),value:me.account.username},{label:t('level'),value:me.agent.depth}]" @close="profileOpen=false"><RegistrationLink :share-title="brandContext.brand.name" v-if="registrationLink" :locale="locale" :url="registrationLink" :disabled="false" :label="t('registrationLink')" :copy-label="t('copyLink')" :copied-label="t('linkCopied')" :failed-label="t('copyFailed')" /><p v-else class="muted" :role="me.code?'alert':undefined">{{t(me.code?'registrationLinkUnavailable':'unavailable')}}</p></AccountProfileDialog>
  <ChangePasswordDialog v-if="passwordOpen" v-model:current="currentPassword" v-model:next="newPassword" v-model:confirmation="confirmPassword" :title="t('changePassword')" :close-label="t('close')" :current-label="t('currentPassword')" :new-label="t('newPassword')" :confirm-label="t('confirmPassword')" :cancel-label="t('cancel')" :submit-label="t(passwordPending?'retry':'savePassword')" :busy="passwordBusy" :frozen="Boolean(passwordPending)" @close="passwordOpen=false" :submit="changePassword"><p v-if="passwordValidation" class="form-error" role="alert">{{t('passwordMismatch')}}</p><p v-if="passwordError" class="form-error" role="alert">{{i18n.errorText(passwordError)}}</p><p v-if="passwordPending" class="muted">{{t('unknown')}}</p></ChangePasswordDialog>
  <DetailModal class="agent-dialog" v-if="editor" :title="t(editor==='members'?'addMember':'addAgent')" :close-label="t('close')" @close="!busy&&!pending&&(editor=null)"><form v-write-guard="busy" class="editor am-form" novalidate v-inline-validation @submit.prevent="withGlobalLoading(save)"><fieldset class="am-form" :disabled="busy||Boolean(pending)"><AccountCreateFields v-model:account="draft.username" v-model:password="draft.password" :account-label="t('account')" :password-label="t('password')" :account-required="true" account-pattern="[a-z][a-z0-9_]{2,31}" :password-minlength="6" :password-maxlength="30"><template #password-help>{{t('passwordHelp')}}</template></AccountCreateFields><template v-if="editor==='members'"><MemberDetailsFields v-model:name="draft.display_name" v-model:phone="draft.phone" :name-label="t('name')" :phone-label="t('phone')" /><p class="muted">{{t('memberHelp')}}</p></template><template v-else><AgentRatioField v-model="draft.ratio" :label="t(me.agent.effective_mode==='loss'?'profitShare':'turnover')" :help="t('ratioHelp')" /><label class="am-check"><input v-model="draft.allowChildren" type="checkbox" />{{t('allowChildren')}}</label></template></fieldset><p v-if="error" role="alert" class="message error">{{errorText}}</p><p v-if="pending" class="muted">{{t('unknown')}}</p><WriteFormActions :submit-label="t(pending?'retry':'save')" :cancel-label="t('cancel')" :busy="busy" :frozen="Boolean(pending)" :invalid="!pending&&!formValid" @cancel="editor=null" /></form></DetailModal>
 </ManagementShell>
</template>
