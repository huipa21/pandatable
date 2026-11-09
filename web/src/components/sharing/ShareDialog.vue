<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { errorMessage } from '../../api/client'
import type { Grant, GrantLevel, ResourceKind } from '../../api/types'
import { accessLevelLabel } from '../../i18n'
import { useAuthStore } from '../../stores/auth'
import {
  useSharingStore,
  validShareSearch,
  type ShareChange,
} from '../../stores/sharing'
import DirectGrantRow from './DirectGrantRow.vue'

const { t } = useI18n()
const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const share = useSharingStore()
const dialog = ref<HTMLDialogElement>()
const reviewHeading = ref<HTMLElement>()
const alert = ref<HTMLElement>()
const selectedId = ref('')
const level = ref<GrantLevel>('read')
const selected = computed(() =>
  share.results?.items.find((item) => item.id === selectedId.value),
)
const rank = { read: 1, write: 2, owner: 3 }
const selectedInherited = computed(() => {
  let highest: GrantLevel | null = null
  for (const grant of share.listing?.inherited ?? []) {
    if (
      grant.principalType === share.principalType &&
      grant.principalId === selectedId.value &&
      (!highest || rank[grant.level] > rank[highest])
    )
      highest = grant.level
  }
  return highest
})
const selectedHigher = computed(
  () =>
    selectedInherited.value &&
    rank[selectedInherited.value] > rank[level.value],
)
const privateOnlyYou = computed(() => {
  const listing = share.listing
  const userId = auth.user?.id
  if (!listing || !userId || share.loading || share.busy || share.error)
    return false

  const grants = [...listing.items, ...listing.inherited]
  return (
    grants.some(
      (grant) =>
        grant.principalType === 'user' &&
        grant.principalId === userId &&
        grant.level === 'owner',
    ) &&
    grants.every(
      (grant) => grant.principalType === 'user' && grant.principalId === userId,
    )
  )
})
const searchDisabled = computed(
  () =>
    share.searching ||
    share.busy ||
    !share.editable ||
    !validShareSearch(share.query),
)
const reviewText = computed(() => {
  const change = share.review
  if (!change) return ''
  return change.removeId
    ? t('sharing.confirmRemove', { name: change.name })
    : t('sharing.confirmLevel', {
        name: change.name,
        level: accessLevelLabel(change.level),
      })
})
function principalName(grant: Grant) {
  return (
    grant.principal?.name ??
    t('sharing.missingPrincipal', { id: grant.principalId })
  )
}
function source(kind: ResourceKind) {
  switch (kind) {
    case 'workspace':
      return t('sharing.workspace')
    case 'book':
      return t('sharing.book')
    case 'table':
      return t('sharing.table')
  }
}
function review(change: ShareChange) {
  share.writeError = null
  share.review = change
}
function add() {
  if (!selected.value) return
  review({
    principalType: share.principalType,
    principalId: selected.value.id,
    name: selected.value.name,
    level: level.value,
    inheritedLevel: selectedInherited.value,
  })
}
function cancelReview() {
  share.review = null
  share.writeError = null
}
watch(
  () => share.results,
  () => {
    selectedId.value = ''
    level.value = 'read'
  },
)
watch(
  () => share.target,
  async () => {
    await nextTick()
    if (share.target) {
      if (!dialog.value?.open) dialog.value?.showModal()
    } else dialog.value?.close()
  },
)
watch(
  () => share.review,
  async () => {
    await nextTick()
    reviewHeading.value?.focus()
  },
)
watch(
  () => share.error ?? share.writeError ?? share.searchError,
  async (value) => {
    if (value) {
      await nextTick()
      alert.value?.focus()
    }
  },
)
watch(
  () => route.fullPath,
  () => share.close(true),
)
watch(
  () => auth.user,
  async (user) => {
    if (!user) await router.replace('/login')
    else if (user.mustChangePassword) await router.replace('/change-password')
  },
)
onBeforeUnmount(() => share.close(true))
</script>

<template>
  <dialog
    ref="dialog"
    class="share-dialog"
    :aria-label="t('sharing.title')"
    @cancel.prevent="share.close()"
  >
    <template v-if="share.target">
      <header class="admin-heading">
        <h2>{{ t('sharing.resourceTitle', { name: share.target.name }) }}</h2>
        <button autofocus :disabled="share.busy" @click="share.close()">
          {{ t('sharing.close') }}
        </button>
      </header>
      <p>{{ t('sharing.inheritanceNotice') }}</p>
      <p v-if="share.target.kind === 'workspace'">
        {{ t('sharing.lastOwnerNotice') }}
      </p>
      <p v-if="privateOnlyYou" class="private-state" role="status">
        {{ t('wmb.privateOnlyYou') }}
      </p>
      <p v-if="share.loading || share.busy" role="status">
        {{ share.busy ? t('sharing.saving') : t('sharing.loading') }}
      </p>
      <div
        v-if="share.error || share.writeError || share.searchError"
        ref="alert"
        role="alert"
        tabindex="-1"
      >
        <p>
          {{
            errorMessage(share.error ?? share.writeError ?? share.searchError)
          }}
        </p>
        <button
          v-if="share.error"
          :disabled="share.loading || share.busy"
          @click="share.load()"
        >
          {{ t('auth.retry') }}
        </button>
        <button
          v-else-if="share.searchError"
          :disabled="share.searching || !share.editable"
          @click="share.search()"
        >
          {{ t('sharing.search') }}
        </button>
      </div>
      <section v-if="share.review" class="share-review">
        <h3 ref="reviewHeading" tabindex="-1">{{ t('sharing.review') }}</h3>
        <p>{{ reviewText }}</p>
        <p>
          {{
            share.review.principalType === 'user'
              ? t('sharing.user')
              : t('sharing.group')
          }}
        </p>
        <p v-if="share.review.inheritedLevel">
          {{
            t('sharing.inheritedRemains', {
              level: accessLevelLabel(share.review.inheritedLevel),
            })
          }}
        </p>
        <p>{{ t('sharing.selfNotice') }}</p>
        <div class="nav-actions">
          <button
            :disabled="share.busy || !share.editable"
            @click="share.commit()"
          >
            {{ t('admin.confirm') }}
          </button>
          <button :disabled="share.busy" @click="cancelReview">
            {{ t('admin.cancel') }}
          </button>
        </div>
      </section>
      <template v-else-if="share.listing">
        <section :aria-label="t('sharing.direct')">
          <h3>{{ t('sharing.direct') }}</h3>
          <p v-if="share.listing.items.length === 0">
            {{ t('sharing.emptyDirect') }}
          </p>
          <ul class="share-list">
            <DirectGrantRow
              v-for="grant in share.listing.items"
              :key="grant.id"
              :grant="grant"
              :disabled="share.busy || !share.editable"
              @review="review"
            />
          </ul>
        </section>
        <section :aria-label="t('sharing.inherited')">
          <h3>{{ t('sharing.inherited') }}</h3>
          <p v-if="share.listing.inherited.length === 0">
            {{ t('sharing.emptyInherited') }}
          </p>
          <ul class="share-list">
            <li
              v-for="grant in share.listing.inherited"
              :key="grant.id"
              class="share-grant"
            >
              <p>
                {{ principalName(grant) }}
                <span v-if="grant.principal?.username">{{
                  t('sharing.username', { username: grant.principal.username })
                }}</span>
                <span>{{
                  grant.principalType === 'user'
                    ? t('sharing.user')
                    : t('sharing.group')
                }}</span>
                <span v-if="grant.principal?.status === 'disabled'">{{
                  t('sharing.disabled')
                }}</span>
              </p>
              <p>
                {{
                  t('sharing.source', {
                    scope: source(grant.resourceType),
                    id: grant.resourceId,
                    level: accessLevelLabel(grant.level),
                  })
                }}
              </p>
            </li>
          </ul>
        </section>
        <section :aria-label="t('sharing.add')">
          <h3>{{ t('sharing.add') }}</h3>
          <form class="share-search" @submit.prevent="share.search()">
            <label>
              <span>{{ t('sharing.principalType') }}</span>
              <select
                v-model="share.principalType"
                :aria-label="t('sharing.principalType')"
                :disabled="share.busy || !share.editable"
              >
                <option value="user">{{ t('sharing.user') }}</option>
                <option value="group">{{ t('sharing.group') }}</option>
              </select>
            </label>
            <label>
              <span>{{ t('sharing.query') }}</span>
              <input
                v-model="share.query"
                :aria-label="t('sharing.query')"
                :disabled="share.busy || !share.editable"
                :aria-describedby="'share-search-policy'"
              />
            </label>
            <p id="share-search-policy">{{ t('sharing.searchPolicy') }}</p>
            <button type="submit" :disabled="searchDisabled">
              {{ t('sharing.search') }}
            </button>
          </form>
          <p v-if="share.searching" role="status">
            {{ t('sharing.searching') }}
          </p>
          <template v-if="share.results">
            <p v-if="share.results.items.length === 0">
              {{ t('sharing.emptySearch') }}
            </p>
            <p v-if="share.results.hasMore">{{ t('sharing.moreResults') }}</p>
            <form
              v-if="share.results.items.length"
              class="share-search"
              @submit.prevent="add"
            >
              <label>
                <span>{{ t('sharing.selectPrincipal') }}</span>
                <select
                  v-model="selectedId"
                  :aria-label="t('sharing.selectPrincipal')"
                  :disabled="share.busy || !share.editable"
                >
                  <option value="" disabled>
                    {{ t('sharing.selectPrincipal') }}
                  </option>
                  <option
                    v-for="item in share.results.items"
                    :key="item.id"
                    :value="item.id"
                  >
                    {{ item.name
                    }}{{
                      item.username
                        ? t('sharing.username', { username: item.username })
                        : ''
                    }}
                  </option>
                </select>
              </label>
              <label>
                <span>{{ t('sharing.level') }}</span>
                <select
                  v-model="level"
                  :aria-label="t('sharing.level')"
                  :disabled="share.busy || !share.editable"
                >
                  <option value="read">{{ t('nav.level.read') }}</option>
                  <option value="write">{{ t('nav.level.write') }}</option>
                  <option value="owner">{{ t('nav.level.owner') }}</option>
                </select>
              </label>
              <p v-if="selectedHigher">
                {{
                  t('sharing.inheritedHigher', {
                    level: accessLevelLabel(selectedInherited!),
                  })
                }}
              </p>
              <button
                type="submit"
                :disabled="!selected || share.busy || !share.editable"
              >
                {{ t('sharing.review') }}
              </button>
            </form>
          </template>
        </section>
      </template>
    </template>
  </dialog>
</template>
