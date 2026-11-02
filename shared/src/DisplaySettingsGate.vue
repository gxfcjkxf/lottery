<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { withGlobalLoading } from './global-loading'
import { displaySettingsCatalog, loadPlatformDisplaySettings } from './display-settings'
import LoadingOverlay from './LoadingOverlay.vue'
import SiteIcon from './SiteIcon'
const props = defineProps<{ locale: 'en' | 'zh-CN' }>()
const copy = computed(() => displaySettingsCatalog[props.locale])
const ready = ref(false), busy = ref(false), failed = ref(false)
async function load() {
  if (busy.value) return
  busy.value = true
  try { await loadPlatformDisplaySettings(); ready.value = true }
  catch { failed.value = true }
  finally { busy.value = false }
}
onMounted(load)
</script>
<template>
  <slot v-if="ready" />
  <main v-else class="display-settings-gate">
    <section v-if="failed" class="display-settings-gate__card" aria-labelledby="display-settings-gate-title" :aria-busy="busy">
      <span class="display-settings-gate__icon"><SiteIcon name="help" /></span>
      <div role="alert">
        <h1 id="display-settings-gate-title">{{ copy.pageUnavailable }}</h1>
        <p>{{ copy.unavailable }}</p>
      </div>
      <button type="button" class="button button-primary" :disabled="busy" @click="withGlobalLoading(load)">
        <SiteIcon name="refresh" />{{ busy ? copy.loading : copy.retry }}
      </button>
    </section>
    <LoadingOverlay v-else :active="true" :label="copy.loading" />
  </main>
</template>
<style scoped>
.display-settings-gate {
  --control-height: 44px;
  min-height: 100dvh;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  background: var(--canvas);
  color: var(--ink);
  font-size: 14px;
}
.display-settings-gate__card {
  display: grid;
  justify-items: center;
  gap: 20px;
  width: min(100%, 400px);
  min-width: 0;
  padding: 32px 24px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--surface);
  box-shadow: var(--shadow-sm);
  text-align: center;
  overflow-wrap: anywhere;
}
.display-settings-gate__icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius);
  background: var(--line-subtle);
  color: var(--muted);
  --site-icon-size: 26px;
}
.display-settings-gate__card h1 { margin: 0; font-size: 20px; font-weight: 650; line-height: 1.4; }
.display-settings-gate__card p { margin: 8px 0 0; color: var(--muted); line-height: 1.6; }
#app .display-settings-gate__card button { display: inline-flex; align-items: center; justify-content: center; justify-self: center; gap: 8px; min-height: 44px; }
</style>
