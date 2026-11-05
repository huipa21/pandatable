<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, useRouter } from 'vue-router'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const loading = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let active = true

async function resolve() {
  const userId = auth.user?.id
  if (!userId || loading.value || !active) return
  const own = new AbortController()
  controller = own
  loading.value = true
  error.value = null
  const current = () =>
    active && !own.signal.aborted && auth.user?.id === userId
  try {
    const workspaceId = await nav.resolvePreference(own.signal)
    if (!current()) return
    let refreshed = await nav.refresh(true)
    if (!refreshed && !nav.error && current()) refreshed = await nav.refresh()
    if (!current()) return
    if (!refreshed) throw nav.error ?? new Error('Navigation refresh failed')
    await router.replace(`/w/${workspaceId}`)
  } catch (cause) {
    if (current()) error.value = cause
  } finally {
    if (controller === own) loading.value = false
  }
}

watch(
  () => auth.user?.id,
  (id) => {
    controller?.abort()
    loading.value = false
    error.value = null
    if (id && active) void resolve()
  },
  { immediate: true },
)
onBeforeRouteLeave(() => {
  active = false
  controller?.abort()
})
onBeforeUnmount(() => {
  active = false
  controller?.abort()
})
</script>

<template>
  <section class="home-page">
    <p v-if="loading" role="status">
      {{ t('workspaceGrid.ux02.resolving') }}
    </p>
    <div v-else-if="error" role="alert">
      <h1>{{ t('workspaceGrid.ux02.resolveError') }}</h1>
      <p>{{ errorMessage(error) }}</p>
      <button @click="resolve">{{ t('auth.retry') }}</button>
    </div>
  </section>
</template>
