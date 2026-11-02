<script setup lang="ts">
import { onMounted, watch } from 'vue'
import SiteIcon from '@lottery/shared/SiteIcon'
import { useUserI18n } from './i18n'
const props = defineProps<{ retry?: () => void }>()
const { t, locale } = useUserI18n()
function setTitle() { document.title = t('user.site.unavailable') }
onMounted(() => {
  setTitle()
  document.querySelector('meta[name="description"]')?.remove()
  document.querySelectorAll('link[rel="manifest"],link[rel="apple-touch-icon"]').forEach(link => link.remove())
  const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (icon) icon.href = 'data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"/%3E'
  const robots = document.createElement('meta')
  robots.name = 'robots'; robots.content = 'noindex,nofollow'; document.head.append(robots)
})
watch(locale, setTitle)
</script>
<template>
  <main class="site-unavailable">
    <section class="site-unavailable__status" aria-labelledby="site-unavailable-title">
      <span class="site-unavailable__icon"><SiteIcon name="link" /></span>
      <h1 id="site-unavailable-title">{{ t('user.site.unavailable') }}</h1>
      <button v-if="props.retry" type="button" class="button button-secondary" @click="props.retry">{{ t('user.site.retry') }}</button>
    </section>
  </main>
</template>
<style scoped>
.site-unavailable { min-height:100dvh;box-sizing:border-box;display:grid;place-items:center;padding:24px;background:var(--canvas);color:var(--ink); }
.site-unavailable__status { display:grid;justify-items:center;gap:16px;max-width:100%;text-align:center; }
.site-unavailable__icon { display:grid;place-items:center;width:48px;height:48px;border:1px solid var(--line);border-radius:var(--radius);background:var(--surface);color:var(--ink-secondary); }
.site-unavailable h1 { margin:0;font-size:24px;line-height:1.35;font-weight:650;overflow-wrap:anywhere; }
</style>
