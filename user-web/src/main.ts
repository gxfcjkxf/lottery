import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import Page from './Page.vue'
import '@lottery/shared/styles.css'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Page },
    { path: '/login', component: Page }, { path: '/register', component: Page },
    { path: '/account', component: Page },
    { path: '/terms', component: Page }, { path: '/privacy', component: Page },
    { path: '/games/:gameId', component: Page }, { path: '/games/:gameId/bet', component: Page },
    { path: '/bet/confirm', component: Page }, { path: '/orders', component: Page }, { path: '/orders/:id', component: Page },
    { path: '/results', component: Page }, { path: '/wallet', component: Page }, { path: '/wallet/ledger', component: Page },
    { path: '/recharge', component: Page }, { path: '/withdraw', component: Page }, { path: '/notifications', component: Page }, { path: '/help', component: Page },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

createApp(App).use(router).mount('#app')

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js')
  })
}
