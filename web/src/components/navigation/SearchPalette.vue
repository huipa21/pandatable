<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useNavStore } from '../../stores/nav'
import AppIcon from '../AppIcon.vue'

interface Entry {
  key: string
  kind: 'workspace' | 'book' | 'table' | 'view'
  name: string
  context: string
  path: string
}

const { t } = useI18n()
const nav = useNavStore()
const route = useRoute()
const router = useRouter()
const dialog = ref<HTMLDialogElement>()
const input = ref<HTMLInputElement>()
const opened = ref(false)
const query = ref('')
const active = ref(0)
const kindLabel = computed(() => ({
  workspace: t('search.kinds.workspace'),
  book: t('search.kinds.book'),
  table: t('search.kinds.table'),
  view: t('search.kinds.view'),
}))

const entries = computed<Entry[]>(() =>
  nav.items.flatMap((workspace) => [
    {
      key: workspace.id,
      kind: 'workspace' as const,
      name: workspace.name,
      context: '',
      path: workspace.path,
    },
    ...workspace.children.flatMap((book) => [
      {
        key: book.id,
        kind: 'book' as const,
        name: book.name,
        context: workspace.name,
        path: book.path,
      },
      ...book.children.flatMap((table) => [
        {
          key: table.id,
          kind: 'table' as const,
          name: table.name,
          context: `${workspace.name} ${t('nav.separator')} ${book.name}`,
          path: table.path,
        },
        ...table.views.map((view) => ({
          key: view.id,
          kind: 'view' as const,
          name: view.name,
          context: `${book.name} ${t('nav.separator')} ${table.name}`,
          path: `/t/${table.id}/v/${view.id}`,
        })),
      ]),
    ]),
  ]),
)
const results = computed(() => {
  const needle = query.value.trim().toLocaleLowerCase()
  const matches = needle
    ? entries.value.filter((entry) =>
        entry.name.toLocaleLowerCase().includes(needle),
      )
    : entries.value
  return matches.slice(0, 50)
})
watch(results, () => (active.value = 0))

async function open() {
  if (opened.value) return
  query.value = ''
  opened.value = true
  if (nav.loaded === false) void nav.refresh()
  await nextTick()
  dialog.value?.showModal()
  input.value?.focus()
}
function close() {
  dialog.value?.close()
  opened.value = false
}
async function go(entry: Entry | undefined) {
  if (!entry) return
  close()
  await router.push(entry.path)
}
function keydown(event: KeyboardEvent) {
  const count = results.value.length
  if (event.key === 'ArrowDown' && count) {
    event.preventDefault()
    active.value = (active.value + 1) % count
  } else if (event.key === 'ArrowUp' && count) {
    event.preventDefault()
    active.value = (active.value - 1 + count) % count
  } else if (event.key === 'Enter') {
    event.preventDefault()
    void go(results.value[active.value])
  }
}
function shortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    void open()
  }
}
watch(() => route.fullPath, close)
document.addEventListener('keydown', shortcut)
onBeforeUnmount(() => document.removeEventListener('keydown', shortcut))
</script>

<template>
  <button
    class="icon-button"
    :aria-label="t('search.open')"
    :title="`${t('search.open')} (Ctrl+K)`"
    aria-keyshortcuts="Control+K Meta+K"
    @click="open"
  >
    <AppIcon name="search" />
  </button>
  <dialog
    ref="dialog"
    class="search-dialog"
    :aria-label="t('search.title')"
    @cancel.prevent="close"
    @click="$event.target === dialog && close()"
  >
    <div v-if="opened" class="search-content">
      <label class="search-field">
        <AppIcon name="search" />
        <span class="visually-hidden">{{ t('search.input') }}</span>
        <input
          ref="input"
          v-model="query"
          type="search"
          role="combobox"
          aria-controls="search-results"
          :aria-expanded="results.length > 0"
          :aria-activedescendant="
            results[active] ? `search-${results[active]!.key}` : undefined
          "
          :placeholder="t('search.placeholder')"
          maxlength="100"
          @keydown="keydown"
        />
      </label>
      <ul id="search-results" role="listbox" :aria-label="t('search.title')">
        <li
          v-for="(entry, index) in results"
          :id="`search-${entry.key}`"
          :key="entry.key"
          role="option"
          :aria-selected="index === active"
          :class="{ active: index === active }"
          @click="go(entry)"
          @mousemove="active = index"
        >
          <span class="search-kind">{{ kindLabel[entry.kind] }}</span>
          <span class="search-name">{{ entry.name }}</span>
          <span v-if="entry.context" class="search-context">{{
            entry.context
          }}</span>
        </li>
      </ul>
      <p v-if="!results.length" class="path-only">{{ t('search.empty') }}</p>
      <button class="search-close" @click="close">
        {{ t('search.close') }}
      </button>
    </div>
  </dialog>
</template>
