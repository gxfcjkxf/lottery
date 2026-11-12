<script setup lang="ts">
import {ref} from 'vue'
import DetailModal from './DetailModal.vue'
const mobileOpen=ref(false)
import SiteIcon, { type SiteIconName } from './SiteIcon'
import BrandLockup from './BrandLockup.vue'
import ManagementFrame from './ManagementFrame.vue'
defineProps<{title:string;brandName:string;logoText:string;logoUrl?:string|null;pageTitle:string;navigationLabel:string;moreLabel:string;closeLabel:string;items:{id:string;label:string;shortLabel:string;icon:SiteIconName}[];modelValue:string}>()
defineEmits<{'update:modelValue':[value:string]}>()
</script>
<template>
 <ManagementFrame>
  <template #sidebar><BrandLockup :name="brandName" :logo-text="logoText" :logo-url="logoUrl" :href="`#/${items[0].id}`" @click.prevent="$emit('update:modelValue',items[0].id)"/><nav class="side-nav" :aria-label="navigationLabel"><a v-for="item in items" :key="item.id" class="nav-item" :class="{active:modelValue===item.id}" :aria-current="modelValue===item.id?'page':undefined" :href="`#/${item.id}`" @click.prevent="$emit('update:modelValue',item.id)"><span class="nav-icon"><SiteIcon :name="item.icon" /></span><span>{{item.label}}</span></a></nav></template>
  <template #main><header class="topbar"><div class="breadcrumbs"><span>{{title}}</span><span class="crumb-slash">/</span><b>{{pageTitle}}</b></div><div class="top-actions"><slot name="actions" /></div></header><section class="page-content"><div class="console-page-heading page-heading"><h1 class="console-page-title">{{pageTitle}}</h1><slot name="heading-actions" /></div><slot /></section></template>
  <nav class="mobile-nav" :style="{'--console-nav-count':4}" :aria-label="navigationLabel"><a v-for="item in items.slice(0,3)" :key="item.id" :href="`#/${item.id}`" :class="{active:modelValue===item.id}" :aria-current="modelValue===item.id?'page':undefined" @click.prevent="$emit('update:modelValue',item.id)"><SiteIcon :name="item.icon" /><span>{{item.shortLabel}}</span></a><button type="button" class="text-button" :class="{active:items.slice(3).some(item=>item.id===modelValue)}" :aria-expanded="mobileOpen" @click="mobileOpen=true"><SiteIcon name="menu" /><span>{{moreLabel}}</span></button></nav>
  <DetailModal v-if="mobileOpen" :title="navigationLabel" :close-label="closeLabel" @close="mobileOpen=false"><nav class="side-nav" :aria-label="navigationLabel"><a v-for="item in items.slice(3)" :key="item.id" class="nav-item" :class="{active:modelValue===item.id}" :href="`#/${item.id}`" :aria-current="modelValue===item.id?'page':undefined" @click.prevent="mobileOpen=false;$emit('update:modelValue',item.id)"><SiteIcon :name="item.icon" /><span>{{item.label}}</span></a></nav></DetailModal>
 </ManagementFrame>
</template>
