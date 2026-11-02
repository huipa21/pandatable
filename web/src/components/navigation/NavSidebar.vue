<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { errorMessage, RequestError } from '../../api/client'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import NavContents from './NavContents.vue'
import StructureDialog from './StructureDialog.vue'
import ShareDialog from '../sharing/ShareDialog.vue'
import UserMenu from '../account/UserMenu.vue'
import { usePreferencesStore } from '../../stores/preferences'

const { t } = useI18n()
const nav = useNavStore()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const preferences = usePreferencesStore()
const media = window.matchMedia('(max-width: 767px)')
const mobile = ref(media.matches)
const drawer = ref<HTMLDialogElement>()
let timer: ReturnType<typeof setInterval> | undefined

function refresh() {
  if (document.visibilityState !== 'hidden') void nav.refresh()
}
function retry() {
  nav.mutationError = null
  void nav.refresh(true)
}
function visibility() {
  if (document.visibilityState === 'hidden') {
    if (timer) clearInterval(timer)
    timer = undefined
  } else {
    refresh()
    if (!timer) timer = setInterval(refresh, 30_000)
  }
}
function close() {
  drawer.value?.close()
  nav.drawerOpen = false
}
async function open() {
  await nextTick()
  if (nav.drawerOpen && !drawer.value?.open) drawer.value?.showModal()
}
function resize() {
  close()
  mobile.value = media.matches
}
watch(
  () => nav.drawerOpen,
  (value) => (value ? void open() : close()),
)
watch(() => route.fullPath, close)
watch([() => nav.error, () => nav.mutationError], async (errors) => {
  for (const cause of errors) {
    if (!(cause instanceof RequestError)) continue
    if (cause.code === 'ERR_UNAUTHENTICATED') {
      auth.setUser(null)
      await router.replace('/login')
      break
    }
    if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
      await router.replace('/change-password')
      break
    }
  }
})
visibility()
window.addEventListener('focus', refresh)
document.addEventListener('visibilitychange', visibility)
media.addEventListener('change', resize)
onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  window.removeEventListener('focus', refresh)
  document.removeEventListener('visibilitychange', visibility)
  media.removeEventListener('change', resize)
  nav.reset()
})
</script>

<template>
  <div
    :class="[
      'sidebar-container',
      { collapsed: mobile || preferences.sidebarCollapsed },
    ]"
  >
    <aside
      v-if="!mobile && !preferences.sidebarCollapsed"
      class="sidebar-desktop"
    >
      <NavContents />
      <div class="sidebar-footer"><UserMenu placement="top" /></div>
    </aside>
    <dialog
      ref="drawer"
      class="nav-drawer"
      :aria-label="t('nav.title')"
      @cancel.prevent="close"
      @click="$event.target === drawer && close()"
    >
      <div v-if="nav.drawerOpen" class="drawer-content">
        <button @click="close">{{ t('nav.close') }}</button>
        <NavContents @navigate="close" />
      </div>
    </dialog>
    <div v-if="nav.error || nav.mutationError" class="nav-error" role="alert">
      <p>{{ errorMessage(nav.mutationError ?? nav.error) }}</p>
      <p v-if="nav.error && nav.mutationError">{{ errorMessage(nav.error) }}</p>
      <button :disabled="nav.loading" @click="retry">
        {{ t('auth.retry') }}
      </button>
    </div>
    <StructureDialog />
    <ShareDialog />
  </div>
</template>
