<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../../api/client'
import type {
  AccessLevel,
  CommentListing,
  CommentMutation,
  DirectoryListing,
  DirectoryPrincipal,
  MentionUser,
  RowComment,
} from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { useRealtimeStore } from '../../stores/realtime'

const props = defineProps<{
  tableId: string
  viewId: string
  rowId: string
  identity: string
  level: AccessLevel
  focusComment?: string
}>()
const emit = defineEmits<{ changed: []; schemaChanged: [] }>()
const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const nav = useNavStore()
const realtime = useRealtimeStore()
const container = ref<HTMLElement>()
const textarea = ref<HTMLTextAreaElement>()
const items = ref<RowComment[]>([])
const hasMore = ref(false)
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>(null)
const mutationError = ref<unknown>(null)
const blocked = ref(false)
const focusMissing = ref(false)
const editing = ref<string | null>(null)
const draft = ref('')
const suggestions = ref<DirectoryPrincipal[]>([])
const suggestionsMore = ref(false)
const searchError = ref<unknown>(null)
const searchTerm = ref('')
const searching = ref(false)
const selected = ref(0)
const tooLong = computed(
  () => new TextEncoder().encode(draft.value).length > 4096,
)
const canSubmit = computed(
  () =>
    !!draft.value.trim() && !tooLong.value && !saving.value && !blocked.value,
)

let generation = 0
let searchGeneration = 0
let loadController: AbortController | undefined
let mutationController: AbortController | undefined
let searchController: AbortController | undefined
let searchTimer: ReturnType<typeof setTimeout> | undefined
let mentionStart = 0
let mentionEnd = 0
let lastFocused = ''
let disposed = false

function current(token: number) {
  return !disposed && token === generation && props.identity === auth.user?.id
}

function clearSearch() {
  searchGeneration++
  clearTimeout(searchTimer)
  searchController?.abort()
  searchController = undefined
  searchTerm.value = ''
  searching.value = false
  suggestions.value = []
  suggestionsMore.value = false
  searchError.value = null
  selected.value = 0
}

function handleFailure(cause: unknown) {
  error.value = cause
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    blocked.value = true
    items.value = []
    draft.value = ''
    void router.replace('/change-password')
  } else if ([401, 403, 404].includes(cause.status)) {
    blocked.value = true
    items.value = []
    draft.value = ''
    if (cause.status === 401) {
      auth.setUser(null)
      void router.replace('/login')
    } else {
      void nav.refresh(true)
      emit('schemaChanged')
    }
  }
}

async function focusRequestedComment() {
  const id = props.focusComment
  if (!id || id === lastFocused) return
  await nextTick()
  const target = document.getElementById(`comment-${id}`)
  if (!(target instanceof HTMLElement) || !container.value?.contains(target))
    return
  if (typeof target.scrollIntoView === 'function')
    target.scrollIntoView({ block: 'center' })
  target.focus({ preventScroll: true })
  lastFocused = id
}

async function load(append = false) {
  if (props.identity !== auth.user?.id || (append && loading.value)) return
  loadController?.abort()
  const own = new AbortController()
  loadController = own
  const token = generation
  const params = new URLSearchParams()
  if (append) params.set('offset', String(items.value.length))
  if (props.focusComment && !focusMissing.value)
    params.set('focus', props.focusComment)
  const suffix = params.toString()
  loading.value = true
  error.value = null
  try {
    const result = await request<CommentListing>(
      `/tables/${encodeURIComponent(props.tableId)}/rows/${encodeURIComponent(props.rowId)}/comments${suffix ? `?${suffix}` : ''}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token) || own.signal.aborted) return
    if (append) {
      const known = new Set(items.value.map((item) => item.id))
      items.value = [
        ...items.value,
        ...result.items.filter((item) => !known.has(item.id)),
      ]
    } else items.value = result.items
    hasMore.value = result.hasMore
    blocked.value = false
    await focusRequestedComment()
  } catch (cause) {
    if (current(token) && !own.signal.aborted) {
      if (
        cause instanceof RequestError &&
        cause.code === 'ERR_COMMENT_NOT_FOUND'
      ) {
        focusMissing.value = true
        await load()
      } else handleFailure(cause)
    }
  } finally {
    if (current(token) && loadController === own) loading.value = false
  }
}

function searchAtCaret() {
  const element = textarea.value
  clearSearch()
  if (!element || blocked.value || saving.value) return
  const caret = element.selectionStart
  const prefix = draft.value.slice(0, caret)
  const match = /(?:^|[^A-Za-z0-9._@-])@([A-Za-z0-9._-]{0,32})$/.exec(prefix)
  if (!match) return
  const term = match[1] ?? ''
  mentionStart = caret - term.length - 1
  mentionEnd = caret
  if (term.length < 2) return
  searchTerm.value = term
  searching.value = true
  const token = searchGeneration
  searchTimer = setTimeout(() => {
    const own = new AbortController()
    searchController = own
    void request<DirectoryListing>(
      `/tables/${encodeURIComponent(props.tableId)}/directory/mentions?q=${encodeURIComponent(term)}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
      .then((result) => {
        if (disposed || own.signal.aborted || token !== searchGeneration) return
        suggestions.value = result.items.filter(
          (item) => item.username !== null,
        )
        suggestionsMore.value = result.hasMore
      })
      .catch((cause: unknown) => {
        if (!disposed && !own.signal.aborted && token === searchGeneration)
          searchError.value = cause
      })
      .finally(() => {
        if (!disposed && token === searchGeneration) searching.value = false
      })
  }, 150)
}

async function chooseMention(user: DirectoryPrincipal) {
  if (!user.username) return
  const insertion = `@${user.username} `
  draft.value =
    draft.value.slice(0, mentionStart) +
    insertion +
    draft.value.slice(mentionEnd)
  const caret = mentionStart + insertion.length
  clearSearch()
  await nextTick()
  textarea.value?.focus()
  textarea.value?.setSelectionRange(caret, caret)
}

function onKeydown(event: KeyboardEvent) {
  if (!searchTerm.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    clearSearch()
  } else if (suggestions.value.length && event.key === 'ArrowDown') {
    event.preventDefault()
    selected.value = (selected.value + 1) % suggestions.value.length
  } else if (suggestions.value.length && event.key === 'ArrowUp') {
    event.preventDefault()
    selected.value =
      (selected.value + suggestions.value.length - 1) % suggestions.value.length
  } else if (suggestions.value.length && event.key === 'Enter') {
    event.preventDefault()
    const choice = suggestions.value[selected.value]
    if (choice) void chooseMention(choice)
  }
}

function startEdit(comment: RowComment) {
  editing.value = comment.id
  draft.value = comment.body
  mutationError.value = null
  clearSearch()
  void nextTick(() => textarea.value?.focus())
}

function cancelEdit() {
  editing.value = null
  draft.value = ''
  mutationError.value = null
  clearSearch()
}

async function save() {
  if (!canSubmit.value) return
  const own = new AbortController()
  mutationController = own
  const token = generation
  const id = editing.value
  saving.value = true
  mutationError.value = null
  clearSearch()
  try {
    await request<CommentMutation>(
      id
        ? `/comments/${encodeURIComponent(id)}`
        : `/tables/${encodeURIComponent(props.tableId)}/rows/${encodeURIComponent(props.rowId)}/comments`,
      id ? 'PATCH' : 'POST',
      { body: draft.value },
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token) || own.signal.aborted) return
    cancelEdit()
    await load()
    emit('changed')
  } catch (cause) {
    if (current(token) && !own.signal.aborted) {
      mutationError.value = cause
      if (
        cause instanceof RequestError &&
        [401, 403, 404].includes(cause.status)
      )
        handleFailure(cause)
    }
  } finally {
    if (current(token) && mutationController === own) saving.value = false
  }
}

async function remove(comment: RowComment) {
  if (saving.value || !window.confirm(t('comments.deleteConfirm'))) return
  const own = new AbortController()
  mutationController = own
  const token = generation
  saving.value = true
  mutationError.value = null
  try {
    await request(
      `/comments/${encodeURIComponent(comment.id)}`,
      'DELETE',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (!current(token) || own.signal.aborted) return
    if (editing.value === comment.id) cancelEdit()
    if (props.focusComment === comment.id) {
      const query = { ...route.query }
      delete query.comment
      await router.replace({ path: route.path, query })
      await nextTick()
    }
    await load()
    emit('changed')
  } catch (cause) {
    if (current(token) && !own.signal.aborted) {
      mutationError.value = cause
      if (
        cause instanceof RequestError &&
        [401, 403, 404].includes(cause.status)
      )
        handleFailure(cause)
    }
  } finally {
    if (current(token) && mutationController === own) saving.value = false
  }
}

interface Segment {
  text: string
  user?: MentionUser
  unnotified?: boolean
}

function segments(comment: RowComment): Segment[] {
  const result: Segment[] = []
  const users = new Map(
    comment.mentions.map((user) => [user.username.toLowerCase(), user]),
  )
  let end = 0
  for (const match of comment.body.matchAll(/@[A-Za-z0-9._-]+/g)) {
    const start = match.index
    const previous = start > 0 ? comment.body[start - 1] : ''
    const username = match[0].slice(1)
    if (
      (previous && /[A-Za-z0-9._@-]/.test(previous)) ||
      username.length < 3 ||
      username.length > 32
    )
      continue
    if (start > end) result.push({ text: comment.body.slice(end, start) })
    const user = users.get(username.toLowerCase())
    result.push({ text: match[0], user, unnotified: !user })
    end = start + match[0].length
  }
  if (end < comment.body.length) result.push({ text: comment.body.slice(end) })
  return result
}

function commentLink(comment: RowComment) {
  return {
    path: `/t/${encodeURIComponent(props.tableId)}/v/${encodeURIComponent(props.viewId)}/r/${encodeURIComponent(props.rowId)}`,
    query: { comment: comment.id },
    hash: `#comment-${comment.id}`,
  }
}

function timestamp(value: string) {
  return new Date(value).toLocaleString(locale.value, {
    timeZone: auth.user?.effectiveTimezone,
  })
}

watch(
  [
    () => props.tableId,
    () => props.rowId,
    () => props.identity,
    () => auth.user?.id,
  ],
  ([tableId, rowId, identity, userId], _previous, onCleanup) => {
    generation++
    loadController?.abort()
    mutationController?.abort()
    clearSearch()
    items.value = []
    hasMore.value = loading.value = saving.value = blocked.value = false
    focusMissing.value = false
    error.value = mutationError.value = null
    editing.value = null
    draft.value = ''
    lastFocused = ''
    if (!tableId || !rowId || !identity || identity !== userId) return
    void load()
    onCleanup(
      realtime.subscribe(tableId, (event) => {
        if (event.t === 'sub.denied') {
          generation++
          loadController?.abort()
          mutationController?.abort()
          handleFailure(new RequestError(event.code ?? 'ERR_NOT_FOUND', 404))
        } else if (
          event.t === 'resync' ||
          (['comment.created', 'comment.updated', 'comment.deleted'].includes(
            event.t,
          ) &&
            event.ids?.includes(rowId))
        )
          void load()
      }),
    )
  },
  { immediate: true },
)
watch(
  () => props.focusComment,
  () => {
    lastFocused = ''
    focusMissing.value = false
    void load()
  },
)
onBeforeUnmount(() => {
  disposed = true
  generation++
  loadController?.abort()
  mutationController?.abort()
  clearSearch()
})
</script>

<template>
  <section ref="container" class="comments" :aria-label="t('comments.title')">
    <h3>{{ t('comments.title') }}</h3>
    <p v-if="loading" role="status">{{ t('comments.loading') }}</p>
    <p v-if="focusMissing" role="alert">{{ t('comments.focusMissing') }}</p>
    <div v-if="error" role="alert">
      {{ errorMessage(error) }}
      <span v-if="items.length">{{ t('comments.stale') }}</span>
      <button type="button" @click="load()">{{ t('auth.retry') }}</button>
    </div>
    <p v-if="!loading && !error && !items.length">{{ t('comments.empty') }}</p>
    <ol class="comment-list">
      <li
        v-for="comment in items"
        :id="`comment-${comment.id}`"
        :key="comment.id"
        :data-comment-id="comment.id"
        tabindex="-1"
      >
        <div class="comment-meta">
          <strong>{{ comment.authorName || t('comments.deletedUser') }}</strong>
          <time :datetime="comment.createdAt">{{
            timestamp(comment.createdAt)
          }}</time>
          <span v-if="comment.editedAt">{{ t('comments.edited') }}</span>
        </div>
        <p class="comment-body">
          <template v-for="(segment, index) in segments(comment)" :key="index">
            <RouterLink
              v-if="segment.user"
              :to="commentLink(comment)"
              :title="t('comments.mentionLink', { name: segment.user.name })"
            >
              <span v-text="segment.text" />
            </RouterLink>
            <span v-else-if="segment.unnotified">
              {{ segment.text }}
              <small>{{ t('comments.notNotified') }}</small>
            </span>
            <template v-else>{{ segment.text }}</template>
          </template>
        </p>
        <div class="comment-actions">
          <button
            v-if="comment.authorId === identity"
            type="button"
            :disabled="saving"
            @click="startEdit(comment)"
          >
            {{ t('comments.edit') }}
          </button>
          <button
            v-if="comment.authorId === identity || level === 'owner'"
            type="button"
            :disabled="saving"
            @click="remove(comment)"
          >
            {{ t('comments.delete') }}
          </button>
        </div>
      </li>
    </ol>
    <button
      v-if="hasMore"
      type="button"
      :disabled="loading"
      @click="load(true)"
    >
      {{ t('comments.loadMore') }}
    </button>
    <form v-if="!blocked" class="comment-compose" @submit.prevent="save">
      <label for="comment-draft">
        {{ editing ? t('comments.editCompose') : t('comments.compose') }}
      </label>
      <textarea
        id="comment-draft"
        ref="textarea"
        v-model="draft"
        :disabled="saving"
        :aria-expanded="!!searchTerm"
        :aria-controls="searchTerm ? 'mention-suggestions' : undefined"
        :aria-activedescendant="
          suggestions[selected]
            ? `mention-option-${suggestions[selected]?.id}`
            : undefined
        "
        aria-autocomplete="list"
        role="combobox"
        rows="3"
        @input="searchAtCaret"
        @click="searchAtCaret"
        @keydown="onKeydown"
      />
      <p>{{ t('comments.hint') }}</p>
      <div v-if="searchTerm" class="mention-search">
        <p v-if="searching" role="status">{{ t('comments.loading') }}</p>
        <p v-if="searchError" role="alert">{{ errorMessage(searchError) }}</p>
        <p v-else-if="!searching && !suggestions.length">
          {{ t('comments.noSuggestions') }}
        </p>
        <ul
          v-if="suggestions.length"
          id="mention-suggestions"
          role="listbox"
          :aria-label="t('comments.suggestions')"
        >
          <li
            v-for="(user, index) in suggestions"
            :id="`mention-option-${user.id}`"
            :key="user.id"
            role="option"
            :aria-selected="index === selected"
            @mousedown.prevent
            @click="chooseMention(user)"
          >
            {{
              t('comments.suggestionLabel', {
                name: user.name,
                username: `@${user.username}`,
              })
            }}
          </li>
        </ul>
        <p v-if="suggestionsMore">{{ t('comments.moreSuggestions') }}</p>
      </div>
      <p v-if="tooLong" role="alert">{{ t('comments.limit') }}</p>
      <p v-if="mutationError" role="alert">
        {{ errorMessage(mutationError) }} {{ t('comments.uncertain') }}
        <button type="button" @click="load()">{{ t('auth.retry') }}</button>
      </p>
      <div class="comment-actions">
        <button type="submit" :disabled="!canSubmit">
          {{ editing ? t('comments.save') : t('comments.post') }}
        </button>
        <button
          v-if="editing"
          type="button"
          :disabled="saving"
          @click="cancelEdit"
        >
          {{ t('comments.cancel') }}
        </button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.comments {
  display: grid;
  gap: 10px;
  padding: 16px 0;
  border-top: 1px solid var(--border);
}
.comment-list {
  display: grid;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.comment-list li {
  padding: 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
}
.comment-meta,
.comment-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.comment-body {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.comment-compose {
  display: grid;
  gap: 8px;
}
textarea {
  width: 100%;
  min-width: 0;
  color: var(--text);
  background: var(--surface);
}
.mention-search ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.mention-search li {
  padding: 8px;
  cursor: pointer;
}
.mention-search li[aria-selected='true'] {
  outline: 2px solid var(--border);
}
</style>
