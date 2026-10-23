<script setup lang="ts" generic="T extends string">
import { withGlobalLoading } from './index'
defineProps<{items:Array<{id:T;label:string}>;value:T;disabled?:boolean;select:(value:T)=>Promise<void>}>()
</script>
<template><div class="console-tabs"><button v-for="item in items" :key="item.id" type="button" :class="{selected:value===item.id}" :aria-pressed="value===item.id" :disabled="disabled" @click="withGlobalLoading(select,item.id)">{{item.label}}</button></div></template>
