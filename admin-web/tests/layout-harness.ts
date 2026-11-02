import { createApp, h } from 'vue'
import ConfiguredRuleWorkbench from '../../shared/src/ConfiguredRuleWorkbench.vue'
import PlatformLottery from '../../shared/src/PlatformLottery.vue'
import ConfiguredPublishing from '../../shared/src/ConfiguredPublishing.vue'
import ConfiguredOrdersPanel from '../../shared/src/ConfiguredOrdersPanel.vue'
import {setPlatformDisplaySettings} from '@lottery/shared'
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
import AccessManagement from '../src/AccessManagement.vue'
import AgentManagement from '../src/AgentManagement.vue'
import JoinCodeManagement from '../src/JoinCodeManagement.vue'
import { createAdminI18n, adminI18nKey } from '../src/i18n'
import '../../shared/src/styles.css'
import '../src/style.css'
import '../../shared/src/controls.css'
import '../../shared/src/refresh-button.css'
import '../../shared/src/management.css'
import '../../shared/src/key-value.css'

const { component, props, locale } = JSON.parse(document.getElementById('harness-data')!.textContent!)
const components = { ConfiguredRuleWorkbench, PlatformLottery, ConfiguredPublishing, ConfiguredOrdersPanel, AccessManagement, AgentManagement, JoinCodeManagement }
const i18n = createAdminI18n()
i18n.setLocale(locale)
createApp({ render: () => h('main', { class: 'management-console' }, h(components[component as keyof typeof components], props)) }).provide(adminI18nKey, i18n).mount('#app')
