<script setup lang="ts">
import TablePagination from './TablePagination.vue'
import type { LocaleCode } from './brand'
defineProps<{page:number;count:number;total:string;locale:LocaleCode;pageSize:number;previousLabel:string;nextLabel:string;busy:boolean;hasPrevious:boolean;hasNext:boolean;goTo:(page:number)=>Promise<void>;changePageSize:(size:number)=>unknown;reachablePage?:number}>()
</script>
<template><TablePagination :busy="busy" :change-page-size="changePageSize" :page="page" :count="count" :total="total" :locale="locale" :page-size="pageSize" :previous-label="previousLabel" :next-label="nextLabel" :previous-disabled="busy||!hasPrevious" :next-disabled="busy||!hasNext" :reachable-page="reachablePage" :go-to="goTo" /></template>
