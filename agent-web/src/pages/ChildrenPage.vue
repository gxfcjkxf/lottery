<script setup lang="ts">
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import {ref,onMounted,onBeforeUnmount,computed} from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {associationLabel,vGlobalLoading} from '@lottery/shared'
import type {AgentNode} from '../../../admin-web/src/agents-api'
import type {Page} from '../types'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
const {t,locale}=i18n,emit=defineEmits<{'session-invalid':[]}>()
const data=ref<Page<AgentNode>|null>(null),offset=ref(0),loading=ref(false),error=ref<unknown>(null),errorText=computed(()=>i18n.errorText(error.value));let generation=0
async function load(){const ticket=++generation;loading.value=true;error.value=null;try{const result=await request<Page<AgentNode>>(`/children?limit=20&offset=${offset.value}`);if(ticket===generation)data.value=result}catch(e){if(ticket!==generation)return;error.value=e;if(e instanceof AgentError&&e.status===401)emit('session-invalid')}finally{if(ticket===generation)loading.value=false}}
onMounted(load);onBeforeUnmount(()=>generation++)
</script>
<template><section class="panel console-panel console-stack children-directory"><p v-global-loading v-if="loading" role="status">{{t('loading')}}</p><p v-if="error" class="message error" role="alert">{{errorText}}</p><ManagementTable :entries="data?.items??[]" :columns="[{key:'account',label:t('account')},{key:'level',label:t('level')},{key:'status',label:t('status')}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('empty')"><template #account="{entry}">{{associationLabel(entry,'agent',entry.id,locale)}}</template><template #level="{entry}">{{entry.depth}}</template><template #status="{entry}">{{t(entry.config.status==='active'?'active':'disabled')}}</template></ManagementTable><PaginationControls :summary="t('totalRecords',{count:data?.total_count??'0'})" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+20)<BigInt(data!.total_count)" :previous="async()=>{offset-=20;await load()}" :next="async()=>{offset+=20;await load()}" /></section></template>
