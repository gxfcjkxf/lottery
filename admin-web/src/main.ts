import { createApp, watch } from 'vue'
import { adminI18nKey, createAdminI18n } from './i18n'
import App from './App.vue'
import './style.css'
import './directory.css'

let localeStorage: Storage | null = null
try { localeStorage = window.localStorage } catch { /* The UI also works without browser storage. */ }
const i18n = createAdminI18n(localeStorage)
watch(i18n.locale, locale => { document.documentElement.lang = locale }, { immediate: true })
createApp(App).provide(adminI18nKey, i18n).mount('#app')

if (import.meta.env.PROD && 'serviceWorker' in navigator) window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js'))
