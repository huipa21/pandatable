import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { i18n, loadLocale } from './i18n'
import { router } from './router'
import { usePreferencesStore } from './stores/preferences'
import { useAppSettingsStore } from './stores/appSettings'
import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(i18n)
// Keep error messages available even if a later locale chunk cannot be fetched.
await loadLocale('en')
await usePreferencesStore(pinia).setLocale(
  usePreferencesStore(pinia).locale,
  false,
)
await useAppSettingsStore(pinia).refresh()
app.use(router)
await router.isReady()
app.mount('#app')
