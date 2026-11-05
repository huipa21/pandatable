<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { errorMessage } from '../../api/client'
import { useNavStore } from '../../stores/nav'

const { t } = useI18n()
const nav = useNavStore()

function retry() {
  const refreshNav = !!nav.error || !!nav.mutationError
  nav.mutationError = null
  if (refreshNav) void nav.refresh(true)
  if (nav.preferenceError) void nav.refreshPreference(true)
}
</script>

<template>
  <div
    v-if="nav.error || nav.mutationError || nav.preferenceError"
    class="nav-error"
    role="alert"
  >
    <p v-if="nav.mutationError">{{ errorMessage(nav.mutationError) }}</p>
    <p v-if="nav.error">{{ errorMessage(nav.error) }}</p>
    <p v-if="nav.preferenceError">
      {{ t('workspaceGrid.ux02.preferenceError') }}
      {{ errorMessage(nav.preferenceError) }}
    </p>
    <button
      :disabled="nav.loading || nav.preferenceLoading || nav.preferenceBusy"
      @click="retry"
    >
      {{ t('auth.retry') }}
    </button>
  </div>
</template>
