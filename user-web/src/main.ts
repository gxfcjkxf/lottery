import { setInlineValidationLocale } from '@lottery/shared'
import '@lottery/shared/form-validation.css'
import { createApp, h, watch, shallowRef } from "vue";
import DisplaySettingsGate from '@lottery/shared/DisplaySettingsGate.vue';
import LoadingOverlay from "@lottery/shared/LoadingOverlay.vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import { createAuthClient } from '../../shared/src/auth';
import { createUserSessionGuard } from './session-guard';
import Page from "./Page.vue";
import { createUserI18n, userI18nKey } from "./i18n";
import "@lottery/shared/styles.css";
import "./style.css";
import './workspace.css';
import '@lottery/shared/empty-state.css';
import '@lottery/shared/key-value.css';
import '@lottery/shared/login-entry.css'
import '@lottery/shared/controls.css';
import '@lottery/shared/refresh-button.css';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: Page },
    { path: "/login", component: Page },
    { path: "/register", component: Page },
    { path: "/account", component: Page },
    { path: "/terms", component: Page },
    { path: "/privacy", component: Page },
    { path: "/games", component: Page },
    { path: "/games/:gameId", component: Page },
    { path: "/configured/:gameId", component: Page },
    { path: "/games/:gameId/bet", component: Page },
    { path: "/bet/confirm", component: Page },
    { path: "/orders", component: Page },
    { path: "/orders/:id", component: Page },
    { path: "/results", component: Page },
    { path: "/wallet", component: Page },
    { path: "/wallet/buckets", component: Page },
    { path: "/wallet/ledger", component: Page },
    { path: "/recharge", component: Page },
    { path: "/withdraw", component: Page },
    { path: "/notifications", component: Page },
    { path: "/invites", component: Page },
    { path: "/help", component: Page },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
  scrollBehavior: (_to, _from, savedPosition) => savedPosition ?? { top: 0 },
});

const navigationError = shallowRef<unknown>(null);
let failedTarget = '/';
const sessionClient = createAuthClient({ brandCode: import.meta.env.VITE_BRAND_CODE || undefined });
router.beforeEach(createUserSessionGuard(() => sessionClient.me()));
router.onError((cause, to) => { navigationError.value = cause; failedTarget = to.fullPath; });
router.afterEach((_to, _from, failure) => { if (!failure) navigationError.value = null; });
async function retryNavigation() {
  try { await router.replace(failedTarget); }
  catch (cause) { navigationError.value = cause; }
}

const i18n = createUserI18n();
watch(i18n.locale, locale => { document.documentElement.lang = locale; setInlineValidationLocale(locale) }, { immediate: true });
createApp({ setup: () => () => h(DisplaySettingsGate, { locale: i18n.locale.value }, { default: () => navigationError.value
  ? h('main', { class: 'content' }, [h('section', { class: 'panel' }, [
    h('p', { role: 'alert' }, i18n.errorText(navigationError.value)),
    h('button', { type: 'button', class: 'button button-primary', onClick: retryNavigation }, i18n.t('auth.navigationRetry')),
  ])]) : h(App) }) }).component("GlobalLoadingOverlay", LoadingOverlay).provide(userI18nKey, i18n).use(router).mount("#app");

if ("serviceWorker" in navigator && import.meta.env.PROD) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js", { updateViaCache: "none" });
  });
}
