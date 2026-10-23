<script setup lang="ts">
import { vInlineValidation, withGlobalLoading } from './index'
const query=defineModel<string>({required:true})
defineProps<{ label:string; queryLabel:string; clearLabel:string; busy:boolean; active:boolean; search:()=>Promise<void>; clear:()=>Promise<void> }>()
</script>
<template>
  <form novalidate v-inline-validation class="filters member-filters" @submit.prevent="withGlobalLoading(search)">
    <input v-model="query" class="field search-field" :placeholder="label" :aria-label="label" />
    <slot />
    <button type="submit" class="button button-primary" :disabled="busy">{{queryLabel}}</button>
    <button v-if="active" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(clear)">{{clearLabel}}</button>
  </form>
</template>
