<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { usePreferencesStore } from '../stores/preferences'
import { useAuthStore } from '../stores/auth'
import { useAppSettingsStore } from '../stores/appSettings'
import { useNavStore } from '../stores/nav'
import { useNavLocation } from '../composables/useNavLocation'
import AppIcon from '../components/AppIcon.vue'
import AppBreadcrumbs from '../components/navigation/AppBreadcrumbs.vue'
import NavSidebar from '../components/navigation/NavSidebar.vue'
import SearchPalette from '../components/navigation/SearchPalette.vue'
import UserMenu from '../components/account/UserMenu.vue'

const { t } = useI18n()
const preferences = usePreferencesStore()
const auth = useAuthStore()
const appSettings = useAppSettingsStore()
const nav = useNavStore()
const route = useRoute()
const { routeWorkspaceId } = useNavLocation()
const media = window.matchMedia('(max-width: 767px)')
const mobile = ref(media.matches)
const resize = (event: MediaQueryListEvent) => (mobile.value = event.matches)
media.addEventListener('change', resize)
onBeforeUnmount(() => media.removeEventListener('change', resize))

const navigationVisible = computed(
  () => !!auth.user && !auth.user.mustChangePassword && route.path !== '/setup',
)
const sidebarShown = computed(
  () =>
    navigationVisible.value && !mobile.value && !preferences.sidebarCollapsed,
)
const otherLocale = computed(() =>
  preferences.locale === 'en' ? 'zh-CN' : 'en',
)
const otherLocaleName = computed(() =>
  otherLocale.value === 'en'
    ? t('common.languages.en')
    : t('common.languages.zhCN'),
)

watch(
  routeWorkspaceId,
  (id) => {
    if (id) nav.lastWorkspaceId = id
  },
  { immediate: true },
)

function toggleSidebar() {
  if (mobile.value) nav.drawerOpen = true
  else preferences.toggleSidebar()
}
</script>

<template>
  <div
    :class="[
      'app-shell',
      { 'with-sidebar': navigationVisible, 'sidebar-shown': sidebarShown },
    ]"
  >
    <header class="app-header">
      <div class="header-brand">
        <RouterLink class="brand" :to="auth.user ? '/home' : '/login'">
          <span class="brand-mark" aria-hidden="true">{{
            t('common.mark')
          }}</span>
          <span class="brand-name">{{
            appSettings.appName ?? t('common.appName')
          }}</span>
        </RouterLink>
        <button
          v-if="navigationVisible"
          class="icon-button sidebar-toggle"
          :aria-label="
            mobile
              ? t('nav.open')
              : preferences.sidebarCollapsed
                ? t('nav.expandSidebar')
                : t('nav.collapseSidebar')
          "
          :title="
            mobile
              ? t('nav.open')
              : preferences.sidebarCollapsed
                ? t('nav.expandSidebar')
                : t('nav.collapseSidebar')
          "
          :aria-expanded="
            mobile ? nav.drawerOpen : !preferences.sidebarCollapsed
          "
          @click="toggleSidebar"
        >
          <AppIcon :name="sidebarShown ? 'panelClose' : 'panelOpen'" />
        </button>
      </div>
      <div class="header-main">
        <AppBreadcrumbs v-if="navigationVisible" />
        <div class="preferences" :aria-label="t('common.preferences')">
          <SearchPalette v-if="navigationVisible" />
          <button
            class="icon-button language-toggle"
            :title="t('common.switchLanguage', { name: otherLocaleName })"
            :lang="otherLocale"
            @click="preferences.setLocale(otherLocale)"
          >
            {{ otherLocaleName }}
          </button>
          <button
            class="icon-button"
            :aria-label="
              preferences.isDark ? t('common.useLight') : t('common.useDark')
            "
            :title="
              preferences.isDark ? t('common.useLight') : t('common.useDark')
            "
            @click="preferences.toggleTheme()"
          >
            <AppIcon :name="preferences.isDark ? 'sun' : 'moon'" />
          </button>
          <UserMenu v-if="auth.user && !sidebarShown" />
        </div>
      </div>
    </header>
    <div class="app-body">
      <NavSidebar v-if="navigationVisible && auth.user" :key="auth.user.id" />
      <main class="app-main">
        <slot />
      </main>
    </div>
  </div>
</template>
