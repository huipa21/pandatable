<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage, RequestError } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import NotificationBell from '../notifications/NotificationBell.vue'

withDefaults(defineProps<{ placement?: 'top' | 'bottom' }>(), {
  placement: 'bottom',
})
const { t } = useI18n()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const root = ref<HTMLElement>()
const opened = ref(false)
const unread = ref(0)
const busy = ref(false)
const error = ref<unknown>(null)
const name = computed(() => auth.user?.displayName || auth.user?.username || '')
const initial = computed(() =>
  (Array.from(name.value.trim())[0] ?? '?').toUpperCase(),
)
const admin = computed(() => auth.user?.role === 'admin')
const adminLinks = computed(() => [
  { to: '/admin/dashboard', label: t('dashboard.title') },
  { to: '/admin/users', label: t('admin.users') },
  { to: '/admin/groups', label: t('groups.title') },
  { to: '/admin/settings', label: t('settings.title') },
  { to: '/admin/workspaces', label: t('workspaces.title') },
  { to: '/admin/backups', label: t('backups.title') },
])

function close() {
  opened.value = false
}
function outside(event: PointerEvent) {
  if (opened.value && !root.value?.contains(event.target as Node)) close()
}
function escape(event: KeyboardEvent) {
  if (opened.value && event.key === 'Escape') close()
}
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
watch(() => route.fullPath, close)
document.addEventListener('pointerdown', outside)
document.addEventListener('keydown', escape)
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', outside)
  document.removeEventListener('keydown', escape)
})
</script>

<template>
  <div v-if="auth.user" ref="root" :class="['user-menu', placement]">
    <button
      class="avatar-button"
      :aria-label="
        unread
          ? t('account.menuUnread', { name, count: unread })
          : t('account.menu', { name })
      "
      :aria-expanded="opened"
      aria-haspopup="true"
      @click="opened = !opened"
    >
      <span class="avatar" aria-hidden="true">{{ initial }}</span>
      <span v-if="unread" class="avatar-badge" aria-hidden="true">{{
        Math.min(unread, 99)
      }}</span>
    </button>
    <div v-show="opened" class="user-menu-panel">
      <p class="user-menu-name">{{ name }}</p>
      <template v-if="!auth.user.mustChangePassword">
        <NotificationBell @count="unread = $event" />
        <RouterLink to="/profile">{{ t('profile.title') }}</RouterLink>
        <RouterLink to="/change-password">
          {{ t('auth.changePassword') }}
        </RouterLink>
        <RouterLink to="/trash">{{ t('trash.title') }}</RouterLink>
        <details v-if="admin" class="user-submenu">
          <summary>{{ t('admin.menu') }}</summary>
          <RouterLink v-for="link in adminLinks" :key="link.to" :to="link.to">
            {{ link.label }}
          </RouterLink>
        </details>
      </template>
      <button class="sign-out" :disabled="busy" @click="logout">
        {{ t('auth.signOut') }}
      </button>
      <p v-if="error" role="alert">{{ errorMessage(error) }}</p>
    </div>
  </div>
</template>
