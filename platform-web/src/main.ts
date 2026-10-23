import { setInlineValidationLocale } from '@lottery/shared'
import '@lottery/shared/form-validation.css'
import { createApp, h, watch } from 'vue'
import DisplaySettingsGate from '@lottery/shared/DisplaySettingsGate.vue'
import LoadingOverlay from '@lottery/shared/LoadingOverlay.vue'
import App from './App.vue'
import { createPlatformI18n, platformI18nKey } from './i18n'
import '@lottery/shared/styles.css'
import './style.css'
import '@lottery/shared/empty-state.css'
import '@lottery/shared/key-value.css'
import '@lottery/shared/management.css'
import '@lottery/shared/login-entry.css'
import '@lottery/shared/controls.css'
import '@lottery/shared/refresh-button.css'

const i18n = createPlatformI18n()
watch(i18n.locale, locale => { document.documentElement.lang = locale; setInlineValidationLocale(locale) }, { immediate: true })
const app = createApp({ setup: () => () => h(DisplaySettingsGate, { locale: i18n.locale.value }, { default: () => h(App) }) }).component('GlobalLoadingOverlay', LoadingOverlay).provide(platformI18nKey, i18n)
app.mount('#app')

if (import.meta.env.PROD && 'serviceWorker' in navigator) window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js', { updateViaCache: 'none' }))
