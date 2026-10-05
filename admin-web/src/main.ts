import { createApp } from 'vue'
import App from './App.vue'
import './style.css'
import './directory.css'

createApp(App).mount('#app')

if (import.meta.env.PROD && 'serviceWorker' in navigator) window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js'))
