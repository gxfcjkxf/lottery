import { installInputNormalization } from '@lottery/shared'
import { createApp } from 'vue'
import App from './App.vue'
import '@lottery/shared/styles.css'
import '@lottery/shared/management.css'
import '@lottery/shared/empty-state.css'
import '@lottery/shared/form-validation.css'
import '@lottery/shared/key-value.css'
import '@lottery/shared/login-entry.css'
import '@lottery/shared/controls.css'
import '@lottery/shared/refresh-button.css'
const stopInputNormalization = installInputNormalization(document)
if (import.meta.hot) import.meta.hot.dispose(stopInputNormalization)

createApp(App).mount('#app')

import "@lottery/shared/themes.css"
