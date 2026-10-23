import { createApp } from 'vue'
import NotificationsPanel from '../src/NotificationsPanel.vue'
import CommissionPanel from '../src/CommissionPanel.vue'
import PlatformSettingsPanel from '../src/PlatformSettingsPanel.vue'
import { createPlatformI18n, platformI18nKey } from '../src/i18n'
import '../../shared/src/styles.css'
import '../src/style.css'
import '../../shared/src/controls.css'
import '../../shared/src/management.css'

const { component, props, locale } = JSON.parse(document.getElementById('harness-data')!.textContent!)
const components = { NotificationsPanel, CommissionPanel, PlatformSettingsPanel }
const i18n = createPlatformI18n()
i18n.setLocale(locale)
createApp(components[component as keyof typeof components], props).provide(platformI18nKey, i18n).mount('#app')
