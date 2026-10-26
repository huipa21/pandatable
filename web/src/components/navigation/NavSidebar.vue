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

const { t } = useI18n()
const nav = useNavStore()
const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const media = window.matchMedia('(max-width: 767px)')
const mobile = ref(media.matches)
const drawer = ref<HTMLDialogElement>()
const drawerOpen = ref(false)
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
  drawerOpen.value = false
}
async function open() {
  drawerOpen.value = true
  await nextTick()
  drawer.value?.showModal()
}
function resize() {
  close()
  mobile.value = media.matches
}
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
  <div class="sidebar-container">
    <button
      v-if="mobile"
      class="drawer-toggle"
      :aria-expanded="drawerOpen"
      @click="open"
    >
      {{ t('nav.open') }}
    </button>
    <aside v-else class="sidebar-desktop"><NavContents /></aside>
    <dialog
      ref="drawer"
      class="nav-drawer"
      :aria-label="t('nav.title')"
      @cancel.prevent="close"
      @click="$event.target === drawer && close()"
    >
      <div v-if="drawerOpen" class="drawer-content">
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
