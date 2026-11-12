<script setup lang="ts">
import { ref } from 'vue'
import ManagementTable from '../../shared/src/ManagementTable.vue'
import ThemeSwitcher from '../../shared/src/ThemeSwitcher.vue'
const props = defineProps<{client:string}>()
const search=ref('')
</script>
<template>
<div :class="client === 'user' ? 'workspace-frame app-frame' : 'management-console app-shell'">
<main class="main-shell" style="margin:0;width:100%">
<header class="topbar workspace-topbar"><b>账户记录</b><div class="top-actions"><ThemeSwitcher locale="zh-CN" /></div></header>
<section class="page-content content">
<section class="panel user-record-list">
<div class="ui-filter-bar"><input v-model="search" aria-label="账号" placeholder="账号" /><button type="button" class="button button-primary">查询</button></div>
<ManagementTable :entries="[{id:'1',account:'test_member',amount:'100 USDT',state:'已到账'}]" :columns="[{key:'account',label:'账号'},{key:'amount',label:'金额'},{key:'state',label:'状态'}]" :empty="false" empty-label="暂无记录"><template #account="{entry}">{{ entry.account }}</template><template #amount="{entry}">{{ entry.amount }}</template><template #state="{entry}">{{ entry.state }}</template></ManagementTable>
</section>
</section>
</main></div>
</template>
