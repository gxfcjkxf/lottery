import { createApp } from 'vue'
import AccessManagement from '../src/AccessManagement.vue'
import AgentManagement from '../src/AgentManagement.vue'
import JoinCodeManagement from '../src/JoinCodeManagement.vue'
import { createAdminI18n, adminI18nKey } from '../src/i18n'
import '../../shared/src/styles.css'
import '../src/style.css'
import '../../shared/src/controls.css'
import '../../shared/src/management.css'

const { component, props, locale } = JSON.parse(document.getElementById('harness-data')!.textContent!)
const components = { AccessManagement, AgentManagement, JoinCodeManagement }
const i18n = createAdminI18n()
i18n.setLocale(locale)
createApp(components[component as keyof typeof components], props).provide(adminI18nKey, i18n).mount('#app')
