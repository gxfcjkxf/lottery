<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import DetailModal from '@lottery/shared/DetailModal.vue'
import SiteIcon from '@lottery/shared/SiteIcon'
import ContactItem from './ContactItem.vue'
import { createApiClient, vGlobalLoading, withGlobalLoading } from '@lottery/shared'
import { isBrandContactConfig, type BrandContactConfig } from '../../shared/src/brand-contacts'
import { useUserI18n } from './i18n'
const props = defineProps<{ brandId: string; brandCode?: string; pageActive: boolean }>()
const { t, errorText } = useUserI18n()
const config = ref<BrandContactConfig | null>(null), dialogOpen = ref(false)
const loading = ref(false), error = ref('')
const supportButton = ref<HTMLButtonElement | null>(null)
const position = ref<{ x: number; y: number } | null>(null)
const dragging = ref(false)
let gesture: { id: number; x: number; y: number; left: number; top: number } | null = null
let moved = false
function moveTo(x: number, y: number) {
  const button = supportButton.value!
  position.value = {
    x: Math.max(12, Math.min(x, window.innerWidth - button.offsetWidth - 12)),
    y: Math.max(12, Math.min(y, window.innerHeight - button.offsetHeight - 12)),
  }
}
function startDrag(event: PointerEvent) {
  if (!event.isPrimary || event.button !== 0) return
  const button = supportButton.value!, box = button.getBoundingClientRect()
  gesture = { id: event.pointerId, x: event.clientX, y: event.clientY, left: box.left, top: box.top }
  moved = false
  button.setPointerCapture(event.pointerId)
}
function drag(event: PointerEvent) {
  if (!gesture || gesture.id !== event.pointerId) return
  const dx = event.clientX - gesture.x, dy = event.clientY - gesture.y
  if (!moved && Math.hypot(dx, dy) < 5) return
  moved = true; dragging.value = true
  moveTo(gesture.left + dx, gesture.top + dy)
}
function endDrag(event: PointerEvent) {
  if (gesture?.id !== event.pointerId) return
  gesture = null; dragging.value = false
}
function openSupport(event: MouseEvent) {
  if (event.detail === 0 || !moved) dialogOpen.value = true
}
function constrainPosition() {
  if (position.value && supportButton.value) moveTo(position.value.x, position.value.y)
}
onMounted(() => window.addEventListener('resize', constrainPosition))
onUnmounted(() => window.removeEventListener('resize', constrainPosition))
const api = createApiClient({ brandCode: props.brandCode })
const defaultContact = computed(() => config.value?.items.find(item => item.id === config.value?.default_id))
const orderedContacts = computed(() => {
  const items = config.value?.items ?? []
  return defaultContact.value ? [defaultContact.value, ...items.filter(item => item.id !== defaultContact.value!.id)] : items
})
let generation = 0
async function read() {
  const token = ++generation
  loading.value = true; error.value = ''
  try {
    const base = props.brandCode ? `/api/v1/b/${encodeURIComponent(props.brandCode)}` : '/api/v1'
    const result = await api.request<unknown>(`${base}/brand-contacts`)
    if (!isBrandContactConfig(result)) throw new Error(t('user.contacts.invalidResponse'))
    if (token === generation) config.value = result
  } catch (cause) { if (token === generation) error.value = errorText(cause) }
  finally { if (token === generation) loading.value = false }
}
watch(() => props.brandId, () => { config.value = null; dialogOpen.value = false; void read() }, { immediate: true })
watch(() => props.pageActive, () => dialogOpen.value = false)
onUnmounted(() => generation++)
</script>
<template>
  <section v-if="pageActive" class="page-section brand-contact-page" :aria-label="t('user.contacts.title')">
    <p v-if="loading" v-global-loading role="status">{{ t('common.loading') }}</p>
    <div v-else-if="error" class="detail-panel" role="alert"><p>{{ error }}</p><button class="button button-secondary" type="button" @click="withGlobalLoading(read)">{{ t('user.contacts.retry') }}</button></div>
    <div v-else-if="config?.items.length" class="contact-directory">
      <ContactItem v-for="item in orderedContacts" :key="item.id" :item="item" />
    </div>
    <p v-else class="empty-state">{{ t('user.contacts.empty') }}</p>
  </section>
  <button v-if="defaultContact" ref="supportButton" type="button" class="customer-service-button" :class="{ 'is-dragging': dragging }" :style="position ? { left: `${position.x}px`, top: `${position.y}px`, insetInlineEnd: 'auto', transform: 'none' } : undefined" :aria-label="t('user.contacts.support')" aria-haspopup="dialog" @pointerdown="startDrag" @pointermove="drag" @pointerup="endDrag" @pointercancel="endDrag" @lostpointercapture="endDrag" @click="openSupport">
    <SiteIcon name="support" /><span>{{ t('user.contacts.support') }}</span>
  </button>
  <DetailModal v-if="dialogOpen" class="ui-form-dialog customer-service-dialog" :title="t('user.contacts.title')" :close-label="t('user.contacts.close')" @close="dialogOpen = false">
    <div><ContactItem v-for="item in orderedContacts" :key="item.id" :item="item" compact /></div>
  </DetailModal>
</template>
<style scoped>
.contact-directory { display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px; }
.customer-service-button { position:fixed;inset-inline-end:max(12px,env(safe-area-inset-right));top:50%;transform:translateY(-50%);z-index:40;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:4px;min-width:56px;min-height:56px;padding:8px;border:1px solid var(--primary);border-radius:var(--radius);background:var(--primary);color:var(--on-primary);box-shadow:var(--shadow-sm);font:inherit;font-size:var(--body-font-size);font-weight:600;cursor:pointer; }
.customer-service-button:focus-visible { outline:2px solid var(--primary);outline-offset:3px; }
.customer-service-button { touch-action:none;user-select:none;cursor:grab; }
.customer-service-button.is-dragging { cursor:grabbing; }
@media(max-width:640px) { .contact-directory { grid-template-columns:minmax(0,1fr);gap:8px; } }
</style>
