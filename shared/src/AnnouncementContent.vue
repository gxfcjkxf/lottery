<script setup lang="ts">
import { computed } from 'vue'
import {ApiError} from './api'
import { safeAnnouncementHTML, safeAnnouncementLink, type AnnouncementCopy } from './announcements'
const props=defineProps<{content:AnnouncementCopy}>()
const html=computed(()=>safeAnnouncementHTML(props.content.html))
const link=computed(()=>{if(props.content.url&&!safeAnnouncementLink(props.content.url))throw new ApiError('Invalid announcement link',400,'ANNOUNCEMENT_INVALID');return props.content.url})
</script>
<template>
 <article class="announcement-content">
  <div class="announcement-body" v-html="html" />
  <a v-if="content.button && content.url" class="button button-primary announcement-cta" :href="link" rel="noopener noreferrer">{{content.button}}</a>
 </article>
</template>
<style scoped>
.announcement-content{display:grid;gap:12px;min-width:0;overflow-wrap:anywhere}.announcement-body{min-width:0;line-height:1.65;color:var(--ink)}.announcement-body :deep(img){display:block;max-width:100%;height:auto;border-radius:var(--radius-sm);margin:8px auto}.announcement-body :deep(:first-child){margin-top:0}.announcement-body :deep(:last-child){margin-bottom:0}.announcement-body :deep(h2){font-size:20px;line-height:1.4}.announcement-body :deep(h3){font-size:18px;line-height:1.4}.announcement-body :deep(a){color:var(--primary);text-decoration:underline}.announcement-body :deep(blockquote){margin:12px 0;padding:8px 12px;border-left:3px solid var(--primary);background:var(--canvas)}.announcement-body :deep(mark){background:var(--warning-bg);color:var(--warning-text)}.announcement-body :deep(ul),.announcement-body :deep(ol){padding-left:24px}.announcement-cta{justify-self:center;max-width:100%;white-space:normal;text-align:center}
</style>
