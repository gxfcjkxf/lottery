<script setup lang="ts">
import { ref, watch } from 'vue'
import SelectControl from '../../shared/src/SelectControl.vue'
import { setInlineValidationLocale, vInlineValidation } from '../../shared/src/form-validation'
import AccountCreateFields from '../../shared/src/AccountCreateFields.vue'
import ChangePasswordDialog from '../../shared/src/ChangePasswordDialog.vue'
const locale = ref<'en'|'zh-CN'>('en')
watch(locale, setInlineValidationLocale, { immediate: true })
const copy = {
  en: { account:'Account', password:'Password', amount:'Points', email:'Email', url:'Source address', status:'Status', terms:'Terms', name:'Display name', notes:'Notes', save:'Save', close:'Close', change:'Change password', current:'Current password', next:'New password', confirm:'Confirm new password' },
  'zh-CN': { account:'账号', password:'密码', amount:'积分', email:'电子邮箱', url:'来源地址', status:'状态', terms:'条款', name:'显示名称', notes:'备注', save:'保存', close:'关闭', change:'修改密码', current:'当前密码', next:'新密码', confirm:'确认新密码' },
}
const account=ref('review_account'),password=ref('password123'),amount=ref('1'),email=ref('name@example.com'),url=ref('https://example.com'),status=ref('active'),terms=ref(true),saves=ref(0)
const displayName=ref('review'),notes=ref('review')
const dialog=ref(false),current=ref('password123'),next=ref(''),confirmation=ref(''),passwordSaves=ref(0)
</script>
<template>
  <main class="validation-harness">
    <button id="language" class="button button-secondary" @click="locale=locale==='en'?'zh-CN':'en'">{{locale}}</button>
    <form novalidate v-inline-validation @submit.prevent="saves++">
      <AccountCreateFields v-model:account="account" v-model:password="password" :account-label="copy[locale].account" :password-label="copy[locale].password" account-required account-pattern="[a-z][a-z0-9_]{2,31}" password-characters />
      <label>{{copy[locale].name}}<input id="display-name" v-model="displayName" maxlength="120" required /></label>
      <label>{{copy[locale].notes}}<textarea id="notes" v-model="notes" maxlength="500" required /></label>
      <label>{{copy[locale].amount}}<input id="amount" v-model="amount" type="number" min="1" max="100" step="0.5" required /></label>
      <label>{{copy[locale].email}}<input id="email" v-model="email" type="email" required /></label>
      <label>{{copy[locale].url}}<input id="url" v-model="url" type="url" required /></label>
      <label>{{copy[locale].status}}<SelectControl id="status" v-model="status" :label="copy[locale].status" required :options="[{value:'',label:''},{value:'active',label:locale==='en'?'Active':'启用'}]" /></label>
      <label class="check"><input id="terms" v-model="terms" type="checkbox" required />{{copy[locale].terms}}</label>
      <button id="save" type="submit" class="button button-primary">{{copy[locale].save}}</button>
      <output id="saves">{{saves}}</output>
    </form>
    <button id="change" class="button button-secondary" @click="dialog=true">{{copy[locale].change}}</button><output id="password-saves">{{passwordSaves}}</output>
    <ChangePasswordDialog v-if="dialog" v-model:current="current" v-model:next="next" v-model:confirmation="confirmation" :title="copy[locale].change" :close-label="copy[locale].close" :current-label="copy[locale].current" :new-label="copy[locale].next" :confirm-label="copy[locale].confirm" :cancel-label="copy[locale].close" :submit-label="copy[locale].save" :busy="false" :submit="async()=>{passwordSaves++;dialog=false}" @close="dialog=false" />
  </main>
</template>
<style scoped>
.validation-harness{width:min(100%,520px);margin:auto;padding:16px;display:grid;gap:12px}.validation-harness form{display:grid;gap:12px;min-width:0}.validation-harness label{display:grid;gap:6px;min-width:0}.validation-harness small{color:var(--muted);line-height:1.5}.validation-harness .check{display:flex;align-items:center;min-height:44px}
</style>
