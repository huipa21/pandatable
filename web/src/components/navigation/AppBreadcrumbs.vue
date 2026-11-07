<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useNavLocation } from '../../composables/useNavLocation'

const { t } = useI18n()
const route = useRoute()
const location = useNavLocation()
const crumbs = computed(() => {
  if (!location.routeWorkspaceId.value && route.path !== '/home') return []
  const items: {
    id: string
    name: string
    path: string
    pathOnly: boolean
    kind: 'root' | 'workspace' | 'book' | 'table' | 'view'
  }[] = [
    {
      id: 'workspaces',
      name: t('workspaceGrid.ux02.workspaces'),
      path: '/home',
      pathOnly: false,
      kind: 'root',
    },
  ]
  if (location.routeWorkspaceId.value) {
    for (const item of [
      location.workspace.value,
      location.book.value,
      location.table.value,
    ])
      if (item) items.push({ ...item, kind: item.kind })
    const table = location.table.value
    const view = location.view.value
    if (table && view)
      items.push({
        id: view.id,
        name: view.name,
        path: `/t/${table.id}/v/${view.id}`,
        pathOnly: false,
        kind: 'view',
      })
  }
  return items
})
</script>

<template>
  <nav
    v-if="crumbs.length"
    class="breadcrumbs"
    :aria-label="t('nav.breadcrumbs')"
  >
    <template v-for="(item, index) in crumbs" :key="item.id">
      <span
        v-if="index"
        class="breadcrumb-separator"
        :class="{
          'breadcrumb-separator-current': index === crumbs.length - 1,
          'breadcrumb-separator-root': index === 1,
        }"
        aria-hidden="true"
      >
        {{ t('nav.separator') }}
      </span>
      <RouterLink
        :to="item.path"
        :class="{
          'breadcrumb-root': item.kind === 'root',
          'breadcrumb-workspace': item.kind === 'workspace',
        }"
        :aria-current="index === crumbs.length - 1 ? 'page' : undefined"
        :title="item.name"
      >
        {{ item.name }}
      </RouterLink>
      <span
        v-if="item.pathOnly"
        class="path-only breadcrumb-path-only"
        :class="{
          'breadcrumb-path-visible': index === 0 || index === crumbs.length - 1,
        }"
      >
        {{ t('nav.pathOnly') }}
      </span>
    </template>
  </nav>
</template>
