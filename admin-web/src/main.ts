import { setInlineValidationLocale } from '@lottery/shared'
import '@lottery/shared/form-validation.css'
import { createApp, h, watch } from 'vue'
import DisplaySettingsGate from '@lottery/shared/DisplaySettingsGate.vue'
import { readLocaleCookie, supportedLocale, writeLocaleCookie } from '@lottery/shared'
import LoadingOverlay from '@lottery/shared/LoadingOverlay.vue'
import { adminI18nKey, createAdminI18n } from './i18n'
import { installVisibleViewport } from './responsive-viewport'
import App from './App.vue'
import '@lottery/shared/styles.css'
import './style.css'
import './directory.css'
import '@lottery/shared/empty-state.css'
import '@lottery/shared/key-value.css'
import '@lottery/shared/management.css'
import '@lottery/shared/login-entry.css'
import '@lottery/shared/controls.css'
import '@lottery/shared/refresh-button.css'

const localeCookie = { getItem: () => readLocaleCookie(), setItem: (_key: string, value: string) => writeLocaleCookie(supportedLocale(value) ?? 'en') }
const i18n = createAdminI18n(localeCookie)
const stopVisibleViewport = installVisibleViewport(window, document)
if (import.meta.hot) import.meta.hot.dispose(stopVisibleViewport)
watch(i18n.locale, locale => { document.documentElement.lang = locale; setInlineValidationLocale(locale) }, { immediate: true })
createApp({ setup: () => () => h(DisplaySettingsGate, { locale: i18n.locale.value }, { default: () => h(App) }) }).component('GlobalLoadingOverlay', LoadingOverlay).provide(adminI18nKey, i18n).mount('#app')

if (import.meta.env.PROD && 'serviceWorker' in navigator) window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js', { updateViaCache: 'none' }))
