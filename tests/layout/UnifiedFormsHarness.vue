<script setup lang="ts">
import {ref} from 'vue'
import DirectorySearch from '../../shared/src/DirectorySearch.vue'
import AccountCreateFields from '../../shared/src/AccountCreateFields.vue'
import WriteFormActions from '../../shared/src/WriteFormActions.vue'
import DetailModal from '../../shared/src/DetailModal.vue'
import SelectControl from '../../shared/src/SelectControl.vue'
const props=defineProps<{locale:'en'|'zh-CN';surface:'user'|'brand'|'platform'|'agent'}>()
const copy={en:{account:'Member account',password:'Password',points:'Points',search:'Search',clear:'Clear',add:'Create member',save:'Save',cancel:'Cancel',close:'Close',status:'Status',all:'All members'},'zh-CN':{account:'会员账号',password:'密码',points:'积分',search:'查询',clear:'清除',add:'创建会员',save:'保存',cancel:'取消',close:'关闭',status:'状态',all:'全部会员'}}[props.locale]
const query=ref(''),account=ref(''),password=ref(''),points=ref(''),status=ref('all'),open=ref(false),searches=ref(0)
async function search(){searches.value++}
async function clear(){query.value='';await search()}
</script>
<template>
 <main class="unified-forms-demo" :class="surface==='user'?'workspace-frame':'management-console'">
  <section class="panel console-panel console-stack">
   <header class="console-page-heading page-heading"><h1 class="console-page-title">{{copy.account}}</h1><div class="console-page-actions"><button class="button button-primary" tabindex="0" @click="open=true">{{copy.add}}</button></div></header>
   <DirectorySearch v-model="query" :label="copy.account" :query-label="copy.search" :clear-label="copy.clear" :busy="false" :active="Boolean(query)" :search="search" :clear="clear" />
   <output data-testid="query-count">{{searches}}</output>
  </section>
  <DetailModal class="ui-form-dialog" v-if="open" :title="copy.add" :close-label="copy.close" @close="open=false">
   <form class="ui-form" @submit.prevent="open=false">
    <AccountCreateFields v-model:account="account" v-model:password="password" :account-label="copy.account" :password-label="copy.password" :account-required="true" />
    <label>{{copy.points}}<input v-model="points" type="number" :aria-label="copy.points" /></label>
    <label>{{copy.status}}<SelectControl v-model="status" :label="copy.status" :options="[{value:'all',label:copy.all}]" /></label>
    <WriteFormActions :submit-label="copy.save" :busy="false" :invalid="!account||!password" />
   </form>
  </DetailModal>
 </main>
</template>
<style scoped>
.unified-forms-demo{box-sizing:border-box;padding:12px;min-width:0}.unified-forms-demo>.panel{padding:12px;border:1px solid var(--line);border-radius:var(--radius);background:var(--surface)}
.unified-forms-demo .page-heading{display:flex;align-items:center;justify-content:space-between;min-width:0;margin:0 0 12px}.unified-forms-demo .console-page-actions{display:flex;justify-content:flex-end;margin-inline-start:auto}.unified-forms-demo .console-page-title{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
