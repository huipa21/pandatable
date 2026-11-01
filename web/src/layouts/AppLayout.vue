<script setup lang="ts">
import { computed, ref } from 'vue'
import { NAlert, NButton, NButtonGroup } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { usePreferencesStore } from '../stores/preferences'
import { useAuthStore } from '../stores/auth'
import { errorMessage, RequestError } from '../api/client'
import { useAppSettingsStore } from '../stores/appSettings'
import { useUndoStore } from '../stores/undo'
import NavSidebar from '../components/navigation/NavSidebar.vue'
import NotificationBell from '../components/notifications/NotificationBell.vue'

const { t } = useI18n()
const preferences = usePreferencesStore()
const auth = useAuthStore()
const appSettings = useAppSettingsStore()
const undo = useUndoStore()
const router = useRouter()
const route = useRoute()
const navigationVisible = computed(
  () => auth.user && !auth.user.mustChangePassword && route.path !== '/setup',
)
const error = ref<unknown>(null)
const busy = ref(false)

async function logout() {
  if (busy.value) return
  busy.value = true
  error.value = null
  try {
    await auth.logout()
    await router.replace('/login')
  } catch (cause) {
    if (cause instanceof RequestError && cause.code === 'ERR_UNAUTHENTICATED') {
      auth.setUser(null)
      await router.replace('/login')
    } else error.value = cause
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="app-shell">
    <header class="app-header">
      <RouterLink class="brand" :to="auth.user ? '/home' : '/login'">
        <span class="brand-mark" aria-hidden="true">{{
          t('common.mark')
        }}</span>
        <span>{{ appSettings.appName ?? t('common.appName') }}</span>
      </RouterLink>
      <div class="preferences" :aria-label="t('common.preferences')">
        <fieldset>
          <legend>{{ t('common.language') }}</legend>
          <NButtonGroup>
            <NButton
              :aria-pressed="preferences.locale === 'en'"
              :type="preferences.locale === 'en' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setLocale('en')"
            >
              {{ t('common.languages.en') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.locale === 'zh-CN'"
              :type="preferences.locale === 'zh-CN' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setLocale('zh-CN')"
            >
              {{ t('common.languages.zhCN') }}
            </NButton>
          </NButtonGroup>
        </fieldset>
        <fieldset>
          <legend>{{ t('common.theme') }}</legend>
          <NButtonGroup>
            <NButton
              :aria-pressed="preferences.theme === 'light'"
              :type="preferences.theme === 'light' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('light')"
            >
              {{ t('common.themes.light') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.theme === 'dark'"
              :type="preferences.theme === 'dark' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('dark')"
            >
              {{ t('common.themes.dark') }}
            </NButton>
            <NButton
              :aria-pressed="preferences.theme === 'system'"
              :type="preferences.theme === 'system' ? 'primary' : 'default'"
              size="small"
              @click="preferences.setTheme('system')"
            >
              {{ t('common.themes.system') }}
            </NButton>
          </NButtonGroup>
        </fieldset>
      </div>
    </header>
    <nav v-if="auth.user" class="session-nav" :aria-label="t('auth.account')">
      <template v-if="!auth.user.mustChangePassword">
        <RouterLink to="/home">{{ t('auth.homeTitle') }}</RouterLink>
        <NButton
          size="small"
          :disabled="!undo.canUndo"
          aria-keyshortcuts="Control+Z Meta+Z"
          @click="undo.undo"
        >
          {{ t('undo.undo') }}
        </NButton>
        <NButton
          size="small"
          :disabled="!undo.canRedo"
          aria-keyshortcuts="Control+Y Meta+Shift+Z"
          @click="undo.redo"
        >
          {{ t('undo.redo') }}
        </NButton>
        <NotificationBell />
        <RouterLink to="/profile">{{ t('profile.title') }}</RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/users">
          {{ t('admin.menu') }}
        </RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/dashboard">
          {{ t('dashboard.title') }}
        </RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/groups">
          {{ t('groups.title') }}
        </RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/settings">
          {{ t('settings.title') }}
        </RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/workspaces">
          {{ t('workspaces.title') }}
        </RouterLink>
        <RouterLink v-if="auth.user.role === 'admin'" to="/admin/backups">
          {{ t('backups.title') }}
        </RouterLink>
        <RouterLink to="/change-password">
          {{ t('auth.changePassword') }}
        </RouterLink>
      </template>
      <NButton size="small" :loading="busy" @click="logout">
        {{ t('auth.signOut') }}
      </NButton>
    </nav>
    <div :class="['app-body', { 'with-sidebar': navigationVisible }]">
      <NavSidebar v-if="navigationVisible && auth.user" :key="auth.user.id" />
      <main class="app-main">
        <NAlert v-if="error" type="error" role="alert">
          {{ errorMessage(error) }}
        </NAlert>
        <slot />
      </main>
    </div>
    <footer class="app-footer">{{ t('common.tagline') }}</footer>
  </div>
</template>
