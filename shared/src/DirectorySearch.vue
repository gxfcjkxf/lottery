<script setup lang="ts">
import { vInlineValidation, withGlobalLoading } from './index'
const query=defineModel<string>({required:true})
defineProps<{ label:string; queryLabel:string; clearLabel:string; busy:boolean; queryPending?:boolean; active:boolean; search:()=>Promise<void>; clear:()=>Promise<void> }>()
</script>
<template>
  <form novalidate v-inline-validation class="filters member-filters ui-filter-bar" @submit.prevent="withGlobalLoading(search)">
    <slot name="input"><input v-model="query" class="field search-field" :placeholder="label" :aria-label="label" /></slot>
    <slot />
    <button type="submit" class="button button-primary" :disabled="busy || queryPending">{{queryLabel}}</button>
    <button v-if="active" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(clear)">{{clearLabel}}</button>
    <slot name="actions" />
  </form>
</template>
