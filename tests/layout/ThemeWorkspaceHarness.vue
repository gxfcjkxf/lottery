<script setup lang="ts">
import { ref } from 'vue'
import ManagementList from '../../shared/src/ManagementList.vue'
import TablePagination from '../../shared/src/TablePagination.vue'
import ManagementTable from '../../shared/src/ManagementTable.vue'
import ThemeSwitcher from '../../shared/src/ThemeSwitcher.vue'
import LanguageSwitcher from '../../shared/src/LanguageSwitcher.vue'
import AccountMenu from '../../shared/src/AccountMenu.vue'
import SiteIcon from '../../shared/src/SiteIcon'
const props = defineProps<{client:string}>()
const search=ref('')
</script>
<template>
<div :class="client === 'user' ? 'workspace-frame app-frame' : 'management-console app-shell'">
<main class="main-shell" style="margin:0;width:100%">
<header class="topbar workspace-topbar"><b>账户记录</b><div class="top-actions" :class="{'workspace-trailing':client==='user'}"><ThemeSwitcher locale="zh-CN" /><LanguageSwitcher model-value="zh-CN" label="语言" :options="[{code:'zh-CN',label:'中文'}]" /><button class="icon-button header-menu" type="button" aria-label="菜单"><SiteIcon name="menu" /></button><AccountMenu v-if="client!=='user'" username="review_member" /></div></header>
<section class="page-content content">
<ManagementList>
<template #filters>
<div class="ui-filter-bar"><input v-model="search" aria-label="账号" placeholder="账号" /><button type="button" class="button button-primary">查询</button></div>
</template>
<ManagementTable :entries="[{id:'1',account:'test_member',amount:'100 USDT',state:'已到账'}]" :columns="[{key:'account',label:'账号'},{key:'amount',label:'金额'},{key:'state',label:'状态'}]" :empty="false" empty-label="暂无记录"><template #account="{entry}">{{ entry.account }}</template><template #amount="{entry}">{{ entry.amount }}</template><template #state="{entry}">{{ entry.state }}</template></ManagementTable>
<template #pagination><TablePagination :page="1" :count="1" total="1" locale="zh-CN" /></template>
</ManagementList>
</section>
</main></div>
</template>
