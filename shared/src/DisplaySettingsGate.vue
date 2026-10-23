<script setup lang="ts">
import { withGlobalLoading } from './global-loading';
import { computed, onMounted, ref } from 'vue'
import { displaySettingsCatalog, loadPlatformDisplaySettings } from './display-settings'
import LoadingOverlay from './LoadingOverlay.vue'
const props = defineProps<{ locale: 'en' | 'zh-CN' }>()
const copy = computed(() => displaySettingsCatalog[props.locale])
const ready = ref(false), busy = ref(false), failed = ref(false)
async function load() {
  busy.value = true; failed.value = false
  try { await loadPlatformDisplaySettings(); ready.value = true }
  catch { failed.value = true }
  finally { busy.value = false }
}
onMounted(load)
</script>
<template>
  <slot v-if="ready" />
  <section v-else class="display-settings-gate">
    <p v-if="failed" role="alert">{{ copy.unavailable }}</p>
    <LoadingOverlay v-else :active="true" :label="copy.loading" />
    <button v-if="failed" type="button" class="button button-secondary" :disabled="busy" @click="withGlobalLoading(load)">{{ copy.retry }}</button>
  </section>
</template>
<style scoped>
.display-settings-gate { padding: 24px; color: var(--ink); font-size: 14px; }
</style>
