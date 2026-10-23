<script setup lang="ts">
import { withGlobalLoading } from './index'
import TablePagination from './TablePagination.vue'
defineProps<{summary:string;previousLabel:string;nextLabel:string;busy:boolean;hasPrevious:boolean;hasNext:boolean;previous:()=>Promise<void>;next:()=>Promise<void>}>()
</script>
<template><TablePagination :summary="summary"><template #actions><button type="button" class="button button-secondary" :disabled="busy||!hasPrevious" @click="withGlobalLoading(previous)">{{previousLabel}}</button><button type="button" class="button button-secondary" :disabled="busy||!hasNext" @click="withGlobalLoading(next)">{{nextLabel}}</button></template></TablePagination></template>
