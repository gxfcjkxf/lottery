<script setup lang="ts">
import ManagementList from "@lottery/shared/ManagementList.vue"
import { useOperationToast } from "@lottery/shared";
import ManagementTable from "@lottery/shared/ManagementTable.vue"
import {ref,onMounted,onBeforeUnmount} from 'vue'
import PaginationControls from '@lottery/shared/PaginationControls.vue'
import {associationLabel,vRefreshButton,vGlobalLoading,withGlobalLoading} from '@lottery/shared'
import type {AgentNode} from '../../../admin-web/src/agents-api'
import type {Page} from '../types'
import {request,AgentError} from '../api'
import {i18n} from '../i18n'
const pageSize = ref(20)

const {t,locale}=i18n,emit=defineEmits<{'session-invalid':[]}>()
const data=ref<Page<AgentNode>|null>(null),offset=ref(0),loading=ref(false),error=ref<unknown>(null);let generation=0
async function load(requestedOffset=offset.value){const ticket=++generation;loading.value=true;error.value=null;try{const result=await request<Page<AgentNode>>(`/children?limit=${pageSize.value}&offset=${requestedOffset}`);if(ticket===generation){data.value=result;offset.value=requestedOffset}}catch(e){if(ticket!==generation)return;if(e instanceof AgentError&&e.status===401){emit('session-invalid');return}error.value=e}finally{if(ticket===generation)loading.value=false}}
onMounted(load);onBeforeUnmount(()=>generation++)

useOperationToast(error, value => i18n.errorText(value), 'error');
</script>
<template><ManagementList :busy="loading" class="children-directory"><template v-if="$slots.actions || error" #actions><slot name="actions" /><button v-if="error" v-refresh-button="t('retryRead')" type="button" class="button button-secondary" :disabled="loading" @click="withGlobalLoading(load)">{{t('retryRead')}}</button></template><p v-global-loading v-if="loading" role="status">{{t('loadingChildren')}}</p><ManagementTable :entries="data?.items??[]" :columns="[{key:'account',label:t('account')},{key:'level',label:t('level')},{key:'status',label:t('status')}]" :empty="!loading&&!error&&data?.items.length===0" :empty-label="t('emptyChildren')"><template #account="{entry}">{{associationLabel(entry,'agent',entry.id,locale)}}</template><template #level="{entry}">{{entry.depth}}</template><template #status="{entry}">{{t(entry.config.status==='active'?'active':'disabled')}}</template></ManagementTable><template #pagination><PaginationControls :total="data?.total_count ?? '0'" :page="Math.floor(offset / pageSize + 1)" :count="data?.items.length ?? 0" :locale="locale" :previous-label="t('previous')" :next-label="t('next')" :busy="loading" :has-previous="offset>0" :has-next="Boolean(data)&&BigInt(offset+pageSize)<BigInt(data!.total_count)"  :page-size="pageSize" :go-to="async target => { await load((target - 1) * pageSize) }" :change-page-size="async (size: number) => { pageSize = size; await load(0) }" /></template></ManagementList></template>
