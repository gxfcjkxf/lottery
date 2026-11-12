<script setup lang="ts">
import ClearableInput from './ClearableInput.vue'
import type { LocaleCode } from './brand'
import { vInlineValidation, withGlobalLoading } from './index'
const query=defineModel<string>({required:true})
defineProps<{ label:string; queryLabel:string; locale:LocaleCode; busy:boolean; queryPending?:boolean; search:()=>Promise<void>; }>()
</script>
<template>
  <form novalidate v-inline-validation class="filters member-filters ui-filter-bar" @submit.prevent="withGlobalLoading(search)">
    <slot name="input"><ClearableInput v-model="query" :locale="locale" :disabled="busy" class="field search-field" :placeholder="label" :aria-label="label" /></slot>
    <slot />
    <button type="submit" class="button button-primary" :disabled="busy || queryPending">{{queryLabel}}</button>
    <slot name="actions" />
  </form>
</template>
