<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'

const { t } = useI18n()
const auth = useAuthStore()
const nav = useNavStore()
</script>

<template>
  <section class="welcome">
    <h1>{{ t('auth.homeTitle') }}</h1>
    <p>{{ t('setup.signedInAs', { name: auth.user?.displayName }) }}</p>
    <p class="welcome-description">{{ t('auth.homeNotice') }}</p>
    <p v-if="nav.loaded && !nav.error && nav.items.length === 0">
      {{ t('nav.empty') }}
    </p>
    <ul class="resource-list">
      <li v-for="item in nav.items" :key="item.id">
        <RouterLink :to="item.path">{{ item.name }}</RouterLink>
        <span v-if="item.pathOnly">{{ t('nav.pathOnly') }}</span>
      </li>
    </ul>
    <button
      :disabled="nav.busy"
      @click="nav.editor = { operation: 'create', kind: 'workspace' }"
    >
      {{ t('nav.create.workspace') }}
    </button>
  </section>
</template>
