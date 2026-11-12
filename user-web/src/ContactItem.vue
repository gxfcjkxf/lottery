<script setup lang="ts">
import { computed, ref } from 'vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import SiteIcon from '@lottery/shared/SiteIcon'
import type { BrandContact } from '../../shared/src/brand-contacts'
import { useUserI18n } from './i18n'

const props = defineProps<{ item: BrandContact; compact?: boolean }>()
const { t } = useUserI18n()
const previewOpen = ref(false)
function openPreview(event: MouseEvent) {
  (event.currentTarget as HTMLButtonElement).focus()
  previewOpen.value = true
}
const link = computed(() => {
  const { item } = props
  if (item.image_url) return null
  if (item.kind === 'telegram' && /^@?[a-zA-Z][a-zA-Z0-9_]{4,31}$/.test(item.value)) return `https://t.me/${item.value.replace(/^@/, '')}`
  if (item.kind === 'whatsapp' && /^\+?[1-9][0-9\s()-]{6,19}$/.test(item.value)) {
    const number = item.value.replace(/\D/g, '')
    if (number.length >= 7 && number.length <= 15) return `https://wa.me/${number}`
  }
  return null
})
</script>

<template>
  <div class="contact-option" :class="compact ? 'contact-compact' : 'detail-panel'">
    <strong class="contact-heading">
      <img v-if="item.kind === 'wechat'" src="/icons/wechat.svg" class="contact-wechat-icon" alt="" aria-hidden="true"/>
      <SiteIcon v-else :name="item.kind === 'telegram' ? 'send' : 'message'" />{{ t(`user.contacts.${item.kind}`) }}
    </strong>
    <button v-if="item.image_url" type="button" class="contact-image-button" :aria-label="t('user.contacts.image', { kind: t(`user.contacts.${item.kind}`) })" aria-haspopup="dialog" @click="openPreview">
    <img :src="item.image_url" :alt="t('user.contacts.image', { kind: t(`user.contacts.${item.kind}`) })" class="contact-thumbnail" />
    </button>
    <a v-else-if="link" :href="link" target="_blank" rel="noopener noreferrer" class="contact-value contact-link">{{ item.value }}</a>
    <span v-else class="contact-value">{{ item.value }}</span>
  </div>
  <DetailModal v-if="previewOpen && item.image_url" class="contact-image-preview" :title="t('user.contacts.image', { kind: t(`user.contacts.${item.kind}`) })" :close-label="t('user.contacts.close')" @close="previewOpen = false">
    <img :src="item.image_url" :alt="t('user.contacts.image', { kind: t(`user.contacts.${item.kind}`) })" class="contact-full-image" />
  </DetailModal>
</template>

<style scoped>
.contact-option { display:grid;align-content:start;gap:12px;width:100%;min-width:0;text-align:start;color:var(--ink); }
.contact-heading { display:flex;align-items:center;gap:8px; }
.contact-wechat-icon { width:20px;height:20px;flex-shrink:0; }
.contact-link { color:var(--primary);text-decoration:underline;text-underline-offset:3px; }
.contact-image-button { display:block;width:120px;max-width:100%;padding:0;border:0;background:transparent;cursor:zoom-in;border-radius:var(--radius-sm); }
.contact-image-button:focus-visible { outline:2px solid var(--primary);outline-offset:3px; }
.contact-full-image { display:block;width:auto;height:auto;max-width:100%;max-height:calc(100dvh - 160px);object-fit:contain;margin-inline:auto; }
.contact-thumbnail { display:block;width:120px;height:120px;max-width:100%;object-fit:contain; }
.contact-value { overflow-wrap:anywhere;white-space:pre-wrap;user-select:text; }
.contact-compact { gap:8px;padding:12px 0;border-bottom:1px solid var(--line); }
.contact-compact:first-child { padding-top:0; }
.contact-compact:last-child { padding-bottom:0;border-bottom:0; }
.contact-compact .contact-image-button { width:30%; }
.contact-compact .contact-thumbnail { width:100%;height:auto; }
</style>
