<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { NAlert, NButton, NInput, NSpin } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { errorMessage, request } from '../../api/client'
import type { AdminUser, UserListing } from '../../api/types'

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{
  'update:modelValue': [value: string]
  failure: [cause: unknown]
}>()
const { t } = useI18n()
const selected = computed({
  get: () => props.modelValue,
  set: (value: string) => emit('update:modelValue', value),
})
const query = ref('')
const applied = ref('')
const offset = ref(0)
const total = ref(0)
const users = ref<AdminUser[]>([])
const loading = ref(false)
const error = ref<unknown>(null)
let controller: AbortController | undefined
let generation = 0

onBeforeUnmount(() => {
  generation++
  controller?.abort()
})

async function load() {
  controller?.abort()
  controller = new AbortController()
  const own = controller
  const current = ++generation
  selected.value = ''
  loading.value = true
  error.value = null
  try {
    const params = new URLSearchParams({
      q: applied.value,
      status: 'active',
      offset: String(offset.value),
      limit: '50',
    })
    const result = await request<UserListing>(
      `/admin/users?${params}`,
      'GET',
      undefined,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
    )
    if (current !== generation) return
    users.value = result.items
    total.value = result.total
  } catch (cause) {
    if (!own.signal.aborted && current === generation) {
      error.value = cause
      emit('failure', cause)
    }
  } finally {
    if (current === generation) loading.value = false
  }
}

function search() {
  applied.value = query.value
  offset.value = 0
  void load()
}

function page(delta: number) {
  offset.value = Math.max(0, offset.value + delta * 50)
  void load()
}

void load()
</script>

<template>
  <div class="member-picker">
    <form class="user-search" @submit.prevent="search">
      <NInput
        v-model:value="query"
        :input-props="{ 'aria-label': t('lifecycle.searchUsers') }"
      />
      <NButton attr-type="submit" :disabled="disabled || loading">
        {{ t('admin.search') }}
      </NButton>
    </form>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <NButton :disabled="disabled || loading" @click="load">
        {{ t('auth.retry') }}
      </NButton>
    </NAlert>
    <NSpin v-if="loading" size="small" />
    <select
      v-model="selected"
      :aria-label="t('lifecycle.selectUser')"
      :disabled="disabled || loading || !!error"
    >
      <option value="">{{ t('lifecycle.selectUser') }}</option>
      <option v-for="user in users" :key="user.id" :value="user.id">
        {{
          t('lifecycle.candidate', {
            name: user.displayName,
            username: user.username,
          })
        }}
      </option>
    </select>
    <p>{{ t('admin.total', { total }) }}</p>
    <div class="user-actions">
      <NButton
        :disabled="disabled || loading || offset === 0"
        @click="page(-1)"
      >
        {{ t('admin.previous') }}
      </NButton>
      <NButton
        :disabled="disabled || loading || offset + 50 >= total"
        @click="page(1)"
      >
        {{ t('admin.next') }}
      </NButton>
    </div>
  </div>
</template>
