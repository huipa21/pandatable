<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { createResourceLabel, accessLevelLabel } from '../i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type { ResourceKind, TableSchema } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { tablePath, useNavStore, type NavigationItem } from '../stores/nav'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const schema = ref<TableSchema | null>(null)
const loading = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let generation = 0
const kind = computed<ResourceKind>(() =>
  route.params.tableId ? 'table' : route.params.bookId ? 'book' : 'workspace',
)
const node = computed(() => {
  const id =
    route.params.tableId ?? route.params.bookId ?? route.params.workspaceId
  return nav.flatItems.find(
    (item) =>
      item.kind === kind.value &&
      item.id === id &&
      (kind.value !== 'book' || item.parentId === route.params.wid),
  )
})
const breadcrumbs = computed(() => {
  const items: NavigationItem[] = []
  let current = node.value
  while (current) {
    items.unshift(current)
    const parentId: string | null = current.parentId
    current = nav.flatItems.find((item) => item.id === parentId)
  }
  return items
})
const displayField = computed(() =>
  schema.value?.fields.find(
    (field) => field.id === schema.value?.table.displayFieldId,
  ),
)
const view = computed(() =>
  schema.value?.views.find((item) => item.id === route.params.viewId),
)
const missing = computed(
  () =>
    nav.loaded &&
    !nav.error &&
    (!node.value || (schema.value && route.params.viewId && !view.value)),
)

async function loadSchema() {
  controller?.abort()
  const current = ++generation
  const own = new AbortController()
  controller = own
  schema.value = null
  error.value = null
  loading.value = false
  if (!node.value || kind.value !== 'table') return
  loading.value = true
  try {
    const result = await request<TableSchema>(
      `/tables/${node.value.id}/schema`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    schema.value = result
    if (!route.params.viewId && result.views.length)
      await router.replace(tablePath(result.table.id, result.views))
  } catch (cause) {
    if (current !== generation || own.signal.aborted) return
    error.value = cause
    if (cause instanceof RequestError) {
      if (cause.code === 'ERR_UNAUTHENTICATED') {
        auth.setUser(null)
        await router.replace('/login')
      } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
        await router.replace('/change-password')
      } else if ([403, 404].includes(cause.status)) await nav.refresh(true)
    }
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(
  [
    () => route.fullPath,
    () => node.value?.id,
    () => node.value?.name,
    () => node.value?.level,
  ],
  () => void loadSchema(),
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>

<template>
  <section class="resource-page">
    <nav
      v-if="!missing && !error"
      class="breadcrumbs"
      :aria-label="t('nav.breadcrumbs')"
    >
      <RouterLink to="/home">{{ t('auth.homeTitle') }}</RouterLink>
      <template v-for="item in breadcrumbs" :key="item.id">
        <span aria-hidden="true">{{ t('nav.separator') }}</span>
        <RouterLink :to="item.path">{{ item.name }}</RouterLink>
        <span v-if="item.pathOnly" class="path-only">{{
          t('nav.pathOnly')
        }}</span>
      </template>
    </nav>
    <p v-if="(!nav.loaded && !nav.error) || loading" role="status">
      {{ t('nav.loading') }}
    </p>
    <div v-else-if="!nav.loaded && nav.error" role="alert">
      <h1>{{ t('nav.unavailable') }}</h1>
      <p>{{ errorMessage(nav.error) }}</p>
      <button :disabled="nav.loading" @click="nav.refresh(true)">
        {{ t('auth.retry') }}
      </button>
    </div>
    <div v-else-if="missing">
      <h1>{{ t('nav.removedTitle') }}</h1>
      <p>{{ t('errors.ERR_NOT_FOUND') }}</p>
      <RouterLink to="/home">{{ t('auth.homeTitle') }}</RouterLink>
    </div>
    <div v-else-if="error" role="alert">
      <h1>{{ t('nav.unavailable') }}</h1>
      <p>{{ errorMessage(error) }}</p>
      <button @click="loadSchema">{{ t('auth.retry') }}</button>
    </div>
    <template v-else-if="node">
      <h1>{{ schema?.table.name ?? node.name }}</h1>
      <p v-if="node.pathOnly">{{ t('nav.pathNotice') }}</p>
      <template v-if="kind !== 'table'">
        <p v-if="node.children.length === 0">
          {{
            kind === 'workspace' ? t('nav.emptyBooks') : t('nav.emptyTables')
          }}
        </p>
        <ul class="resource-list">
          <li v-for="child in node.children" :key="child.id">
            <RouterLink :to="child.path">{{ child.name }}</RouterLink>
            <span v-if="child.pathOnly">{{ t('nav.pathOnly') }}</span>
          </li>
        </ul>
        <button
          v-if="node.canCreate && node.childKind"
          :disabled="nav.busy"
          @click="
            nav.editor = {
              operation: 'create',
              kind: node.childKind,
              parentId: node.id,
            }
          "
        >
          {{ createResourceLabel(node.childKind) }}
        </button>
      </template>
      <template v-else-if="schema">
        <dl class="schema-summary">
          <dt>{{ t('nav.displayField') }}</dt>
          <dd>{{ displayField?.name ?? t('nav.noDisplayField') }}</dd>
          <dt>{{ t('nav.access') }}</dt>
          <dd>{{ accessLevelLabel(schema.myLevel) }}</dd>
        </dl>
        <nav class="view-links" :aria-label="t('nav.views')">
          <RouterLink
            v-for="item in schema.views"
            :key="item.id"
            :to="`/t/${schema.table.id}/v/${item.id}`"
            :aria-current="view?.id === item.id ? 'page' : undefined"
          >
            {{ item.name }}
          </RouterLink>
        </nav>
        <p v-if="schema.views.length === 0">{{ t('nav.emptyViews') }}</p>
        <p>{{ t('nav.tableShellNotice') }}</p>
      </template>
    </template>
    <button
      v-else-if="nav.error"
      :disabled="nav.loading"
      @click="nav.refresh(true)"
    >
      {{ t('auth.retry') }}
    </button>
  </section>
</template>
