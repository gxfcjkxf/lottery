import NotificationTemplatesPanel from '../src/NotificationTemplatesPanel.vue'
import { createApp, h } from 'vue'
import NotificationsPanel from '../src/NotificationsPanel.vue'
import CommissionPanel from '../src/CommissionPanel.vue'
import PlatformSettingsPanel from '../src/PlatformSettingsPanel.vue'
import { createPlatformI18n, platformI18nKey } from '../src/i18n'
import '../../shared/src/styles.css'
import '../src/style.css'
import '../../shared/src/controls.css'
import '../../shared/src/management.css'
import '../../shared/src/refresh-button.css'
import '../../shared/src/key-value.css'

const { component, props, locale } = JSON.parse(document.getElementById('harness-data')!.textContent!)
const components = { NotificationTemplatesPanel, NotificationsPanel, CommissionPanel, PlatformSettingsPanel }
const i18n = createPlatformI18n()
i18n.setLocale(locale)
createApp({ render: () => h('main', { class: 'management-console' }, h(components[component as keyof typeof components], props)) }).provide(platformI18nKey, i18n).mount('#app')
