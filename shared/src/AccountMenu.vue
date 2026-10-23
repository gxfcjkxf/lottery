<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import SiteIcon from './SiteIcon'
defineProps<{ username: string }>()
const root = ref<HTMLElement|null>(null), open = ref(false)
function outside(event: Event) { if (!root.value?.contains(event.target as Node)) open.value = false }
function escape(event: KeyboardEvent) { if (event.key === 'Escape') open.value = false }
onMounted(()=>{document.addEventListener('pointerdown',outside);document.addEventListener('keydown',escape)})
onBeforeUnmount(()=>{document.removeEventListener('pointerdown',outside);document.removeEventListener('keydown',escape)})
</script>
<template>
 <div ref="root" class="account-menu" @keydown.esc.stop="open=false">
  <button class="account-trigger" type="button" :aria-label="username" aria-haspopup="menu" :aria-expanded="open" @click="open=!open"><span class="account-avatar">{{username.slice(0,1).toUpperCase()}}</span><span>{{username}}</span><SiteIcon name="down" /></button>
  <div v-if="open" class="account-popover" role="menu" @click="open=false"><strong>{{username}}</strong><slot /></div>
 </div>
</template>
<style scoped>
.account-menu{position:relative;align-self:stretch;display:flex;align-items:center;min-width:0}
.account-trigger{display:inline-flex;align-items:center;gap:7px;border:0;background:transparent;color:var(--ink);padding:4px 6px;border-radius:var(--radius-sm);font-size:13px;font-weight:600;min-height:36px;max-width:100%}
.account-trigger:hover,.account-trigger:focus-visible{background:var(--canvas)}
.account-avatar{width:24px;height:24px;flex-shrink:0;border-radius:50%;display:grid;place-items:center;background:var(--primary-light);color:var(--primary-dark);font-size:11px}
.account-popover{position:absolute;right:0;top:100%;z-index:40;width:190px;max-width:calc(100vw - 24px);padding:10px;display:grid;gap:4px;background:var(--surface);border:1px solid var(--line);border-radius:var(--radius-sm);box-shadow:var(--shadow-lg);opacity:1;pointer-events:auto;transform:none}
.account-popover strong{overflow-wrap:anywhere;font-size:13px;color:var(--ink)}
.account-popover :deep(button){border:0;background:transparent;color:var(--ink-secondary);text-align:left;padding:8px 6px;border-radius:var(--radius-sm);font-size:13px;min-height:36px}
.account-popover :deep(button:hover){background:var(--canvas)}
.account-popover :deep(.popover-logout){color:var(--danger)}
@media(max-width:900px){.account-trigger>span:nth-child(2){display:none}}
@media(max-width:700px),(pointer:coarse){.account-trigger,.account-popover :deep(button){min-height:44px}}
</style>
