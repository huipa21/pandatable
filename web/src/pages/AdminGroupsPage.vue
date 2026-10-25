<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { NAlert, NButton, NFormItem, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage, request, RequestError } from '../api/client'
import type {
  AdminGroup,
  AdminUser,
  GroupListing,
  GroupMutation,
  UserListing,
} from '../api/types'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const groupId = computed(() => String(route.params.id ?? ''))
const userId = computed(() => String(route.params.userId ?? ''))
const group = ref<AdminGroup | null>(null)
const user = ref<AdminUser | null>(null)
const groups = ref<AdminGroup[]>([])
const members = ref<AdminUser[]>([])
const total = ref(0)
const q = ref('')
const appliedQ = ref('')
const offset = ref(0)
const limit = ref(50)
const loading = ref(false)
const busy = ref(false)
const error = ref<unknown>(null)
const saved = ref(false)
const editing = ref(false)
const deleting = ref(false)
const form = reactive({ name: '', description: '' })
const candidates = ref<AdminUser[]>([])
const candidateQ = ref('')
const candidateSearch = ref('')
const candidateOffset = ref(0)
const candidateTotal = ref(0)
const candidateLoading = ref(false)
const selectedUser = ref('')
let controller: AbortController | undefined
let pickerController: AbortController | undefined
let generation = 0
let pickerGeneration = 0

onBeforeUnmount(() => {
  generation++
  pickerGeneration++
  controller?.abort()
  pickerController?.abort()
})

async function failed(cause: unknown) {
  error.value = cause
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    await router.replace('/login')
  } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    await router.replace('/change-password')
  } else if (cause.code === 'ERR_PERMISSION_DENIED') {
    await router.replace('/home')
  }
}

async function load() {
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  const current = ++generation
  const id = groupId.value
  const uid = userId.value
  loading.value = true
  error.value = null
  const params = new URLSearchParams({
    q: appliedQ.value,
    offset: String(offset.value),
    limit: String(limit.value),
  })
  try {
    if (id) {
      const [detail, list] = await Promise.all([
        request<{ group: AdminGroup }>(
          `/admin/groups/${id}`,
          'GET',
          undefined,
          signal,
        ),
        request<UserListing>(
          `/admin/groups/${id}/members?${params}`,
          'GET',
          undefined,
          signal,
        ),
      ])
      if (current !== generation) return
      group.value = detail.group
      members.value = list.items
      total.value = list.total
    } else {
      const [detail, list] = await Promise.all([
        uid
          ? request<{ user: AdminUser }>(
              `/admin/users/${uid}`,
              'GET',
              undefined,
              signal,
            )
          : Promise.resolve(null),
        request<GroupListing>(
          `${uid ? `/admin/users/${uid}/groups` : '/admin/groups'}?${params}`,
          'GET',
          undefined,
          signal,
        ),
      ])
      if (current !== generation) return
      user.value = detail?.user ?? null
      groups.value = list.items
      total.value = list.total
    }
    if (offset.value > 0 && total.value <= offset.value) {
      offset.value = Math.max(
        0,
        Math.floor((total.value - 1) / limit.value) * limit.value,
      )
      await load()
    }
  } catch (cause) {
    if (!signal.aborted && current === generation) await failed(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}

function search() {
  offset.value = 0
  appliedQ.value = q.value
  void load()
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * limit.value)
  void load()
}

function edit() {
  form.name = group.value?.name ?? ''
  form.description = group.value?.description ?? ''
  editing.value = true
  deleting.value = false
  saved.value = false
}

function confirmDeletion() {
  deleting.value = true
  editing.value = false
}

async function mutate(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = null
  saved.value = false
  try {
    await action()
  } catch (cause) {
    await failed(cause)
  } finally {
    busy.value = false
  }
}

async function save() {
  await mutate(async () => {
    const id = groupId.value
    const result = await request<GroupMutation>(
      id ? `/admin/groups/${id}` : '/admin/groups',
      id ? 'PATCH' : 'POST',
      { ...form },
    )
    editing.value = false
    if (!id) await router.push(`/admin/groups/${result.group.id}`)
    else await load()
    saved.value = true
  })
}

async function removeGroup() {
  await mutate(async () => {
    await request(`/admin/groups/${groupId.value}`, 'DELETE')
    await router.push('/admin/groups')
  })
}

async function loadCandidates() {
  pickerController?.abort()
  pickerController = new AbortController()
  const signal = pickerController.signal
  const current = ++pickerGeneration
  candidateLoading.value = true
  selectedUser.value = ''
  try {
    const params = new URLSearchParams({
      q: candidateSearch.value,
      status: 'active',
      offset: String(candidateOffset.value),
      limit: '50',
    })
    const list = await request<UserListing>(
      `/admin/users?${params}`,
      'GET',
      undefined,
      signal,
    )
    if (current !== pickerGeneration) return
    candidates.value = list.items
    candidateTotal.value = list.total
  } catch (cause) {
    if (!signal.aborted && current === pickerGeneration) await failed(cause)
  } finally {
    if (current === pickerGeneration) candidateLoading.value = false
  }
}

function searchCandidates() {
  candidateOffset.value = 0
  candidateSearch.value = candidateQ.value
  void loadCandidates()
}

function candidatePage(delta: number) {
  candidateOffset.value = Math.max(0, candidateOffset.value + delta * 50)
  void loadCandidates()
}

async function changeMember(id: string, add: boolean) {
  await mutate(async () => {
    await request(
      `/admin/groups/${groupId.value}/members${add ? '' : `/${id}`}`,
      add ? 'POST' : 'DELETE',
      add ? { userId: id } : undefined,
    )
    selectedUser.value = ''
    await load()
    saved.value = true
  })
}

watch(
  () => route.path,
  () => {
    controller?.abort()
    pickerController?.abort()
    pickerGeneration++
    candidateLoading.value = false
    group.value = null
    user.value = null
    groups.value = []
    members.value = []
    candidates.value = []
    total.value = 0
    q.value = ''
    appliedQ.value = ''
    offset.value = 0
    editing.value = false
    deleting.value = false
    saved.value = false
    selectedUser.value = ''
    candidateQ.value = ''
    candidateSearch.value = ''
    candidateOffset.value = 0
    void load()
    if (groupId.value) void loadCandidates()
  },
  { immediate: true },
)
</script>

<template>
  <section class="admin-groups">
    <RouterLink :to="userId ? '/admin/users' : '/admin/groups'">
      {{ userId ? t('groups.backUsers') : t('groups.backGroups') }}
    </RouterLink>
    <div class="admin-heading">
      <h1>
        {{
          groupId
            ? group?.name
            : userId
              ? t('groups.userDetail')
              : t('groups.title')
        }}
      </h1>
      <div v-if="!userId" class="user-actions">
        <NButton
          :disabled="busy || loading || (!!groupId && !group)"
          @click="edit"
        >
          {{ groupId ? t('groups.edit') : t('groups.create') }}
        </NButton>
        <NButton
          v-if="group"
          type="error"
          :disabled="busy"
          @click="confirmDeletion"
        >
          {{ t('groups.delete') }}
        </NButton>
      </div>
    </div>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="busy" @click="load">{{ t('auth.retry') }}</NButton>
    </NAlert>
    <NAlert v-if="saved" type="success" role="status">
      {{ t('groups.saved') }}
    </NAlert>
    <p v-if="group" class="group-description">{{ group.description }}</p>
    <div v-if="user" class="user-details">
      <h2>{{ user.displayName }}</h2>
      <p>{{ user.username }}</p>
      <p v-if="user.email">{{ user.email }}</p>
      <p>
        {{ user.status === 'active' ? t('admin.active') : t('admin.disabled') }}
      </p>
    </div>
    <form v-if="editing" class="group-editor" @submit.prevent="save">
      <h2>{{ groupId ? t('groups.edit') : t('groups.create') }}</h2>
      <NFormItem :label="t('groups.name')">
        <NInput
          v-model:value="form.name"
          :input-props="{ 'aria-label': t('groups.name'), required: true }"
        />
      </NFormItem>
      <NFormItem :label="t('groups.description')">
        <NInput
          v-model:value="form.description"
          type="textarea"
          :input-props="{ 'aria-label': t('groups.description') }"
        />
      </NFormItem>
      <div class="user-actions">
        <NButton
          attr-type="submit"
          type="primary"
          :loading="busy"
          :disabled="busy"
        >
          {{ t('admin.confirm') }}
        </NButton>
        <NButton :disabled="busy" @click="editing = false">
          {{ t('admin.cancel') }}
        </NButton>
      </div>
    </form>
    <NAlert v-if="deleting && group" type="warning" role="alert">
      <p>{{ t('groups.deleteNotice', { name: group.name }) }}</p>
      <div class="user-actions">
        <NButton
          type="error"
          :loading="busy"
          :disabled="busy"
          @click="removeGroup"
        >
          {{ t('admin.confirm') }}
        </NButton>
        <NButton :disabled="busy" @click="deleting = false">
          {{ t('admin.cancel') }}
        </NButton>
      </div>
    </NAlert>
    <h2 v-if="groupId || userId">
      {{ groupId ? t('groups.members') : t('groups.memberships') }}
    </h2>
    <div v-if="group" class="member-picker">
      <form class="user-search" @submit.prevent="searchCandidates">
        <NInput
          v-model:value="candidateQ"
          :input-props="{ 'aria-label': t('admin.searchHint') }"
        />
        <NButton attr-type="submit" :disabled="busy || candidateLoading">
          {{ t('admin.search') }}
        </NButton>
      </form>
      <div class="user-actions">
        <select
          v-model="selectedUser"
          :aria-label="t('groups.selectUser')"
          :disabled="busy || candidateLoading"
        >
          <option value="">{{ t('groups.selectUser') }}</option>
          <option
            v-for="candidate in candidates"
            :key="candidate.id"
            :value="candidate.id"
          >
            {{
              t('groups.candidate', {
                name: candidate.displayName,
                username: candidate.username,
              })
            }}
          </option>
        </select>
        <NButton
          :disabled="busy || candidateLoading || !selectedUser"
          @click="changeMember(selectedUser, true)"
        >
          {{ t('groups.add') }}
        </NButton>
      </div>
      <p>{{ t('admin.total', { total: candidateTotal }) }}</p>
      <div class="user-actions">
        <NButton
          :disabled="busy || candidateLoading || candidateOffset === 0"
          @click="candidatePage(-1)"
        >
          {{ t('admin.previous') }}
        </NButton>
        <NButton
          :disabled="
            busy || candidateLoading || candidateOffset + 50 >= candidateTotal
          "
          @click="candidatePage(1)"
        >
          {{ t('admin.next') }}
        </NButton>
      </div>
    </div>
    <form class="user-search" @submit.prevent="search">
      <NInput
        v-model:value="q"
        :placeholder="groupId ? t('admin.searchHint') : t('groups.searchHint')"
        :input-props="{
          'aria-label': groupId ? t('groups.members') : t('groups.searchHint'),
        }"
      />
      <select
        v-model="limit"
        :aria-label="t('groups.pageSize')"
        @change="search"
      >
        <option :value="10">{{ t('admin.size10') }}</option>
        <option :value="50">{{ t('admin.size50') }}</option>
        <option :value="100">{{ t('admin.size100') }}</option>
        <option :value="500">{{ t('admin.size500') }}</option>
      </select>
      <NButton attr-type="submit" :disabled="busy || loading">
        {{ t('admin.search') }}
      </NButton>
    </form>
    <NSpin v-if="loading" size="small" />
    <p>
      {{
        groupId
          ? t('groups.memberCount', { total })
          : t('groups.total', { total })
      }}
    </p>
    <div class="user-list" :aria-busy="loading">
      <template v-if="groupId">
        <article
          v-for="member in members"
          :key="member.id"
          class="user-card"
          :data-member="member.username"
        >
          <div class="user-details">
            <h3>
              <RouterLink :to="`/admin/users/${member.id}`">
                {{ member.displayName }}
              </RouterLink>
            </h3>
            <p>{{ member.username }}</p>
            <p>
              {{
                member.status === 'active'
                  ? t('admin.active')
                  : t('admin.disabled')
              }}
            </p>
          </div>
          <NButton
            :disabled="busy || loading"
            @click="changeMember(member.id, false)"
          >
            {{ t('groups.remove') }}
          </NButton>
        </article>
      </template>
      <template v-else>
        <article
          v-for="item in groups"
          :key="item.id"
          class="user-card"
          :data-group="item.name"
        >
          <div class="user-details">
            <h2>
              <RouterLink :to="`/admin/groups/${item.id}`">
                {{ item.name }}
              </RouterLink>
            </h2>
            <p class="group-description">{{ item.description }}</p>
            <p>{{ t('groups.memberCount', { total: item.memberCount }) }}</p>
          </div>
        </article>
      </template>
      <p v-if="!loading && !error && !total">
        {{ groupId ? t('groups.emptyMembers') : t('groups.empty') }}
      </p>
    </div>
    <div class="user-actions pagination">
      <NButton :disabled="busy || loading || offset === 0" @click="page(-1)">
        {{ t('admin.previous') }}
      </NButton>
      <NButton
        :disabled="busy || loading || offset + limit >= total"
        @click="page(1)"
      >
        {{ t('admin.next') }}
      </NButton>
    </div>
  </section>
</template>

<style scoped>
.admin-groups {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.admin-groups h1 {
  font-size: 24px;
  overflow-wrap: anywhere;
}
.admin-groups h2 {
  font-size: 19px;
  overflow-wrap: anywhere;
}
.admin-groups h3 {
  font-size: 17px;
  overflow-wrap: anywhere;
}
.group-editor,
.member-picker {
  padding-block: 16px;
  border-block: 1px solid #8885;
  min-width: 0;
}
.group-description {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.member-picker select {
  min-width: 0;
  max-width: 100%;
  width: min(100%, 360px);
}
</style>
