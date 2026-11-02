<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'
import { useNavStore } from '../stores/nav'

const { t } = useI18n()
const auth = useAuthStore()
const nav = useNavStore()
</script>

<template>
  <section class="home-page">
    <h1>{{ t('auth.homeTitle') }}</h1>
    <p>{{ t('setup.signedInAs', { name: auth.user?.displayName }) }}</p>
    <p class="welcome-description">{{ t('auth.homeNotice') }}</p>
    <div class="home-heading">
      <h2>{{ t('nav.chooseWorkspace') }}</h2>
      <button
        class="primary-button"
        :disabled="nav.busy"
        @click="nav.editor = { operation: 'create', kind: 'workspace' }"
      >
        {{ t('nav.create.workspace') }}
      </button>
    </div>
    <p v-if="nav.loading && !nav.loaded" role="status">
      {{ t('nav.loading') }}
    </p>
    <p v-if="nav.loaded && !nav.error && nav.items.length === 0">
      {{ t('nav.empty') }}
    </p>
    <ul class="workspace-cards">
      <li v-for="item in nav.items" :key="item.id" class="workspace-tile">
        <RouterLink :to="item.path" class="workspace-tile-name">
          {{ item.name }}
        </RouterLink>
        <span class="path-only">{{
          t('nav.bookCount', item.children.length)
        }}</span>
        <span v-if="item.pathOnly" class="path-only">{{
          t('nav.pathOnly')
        }}</span>
      </li>
    </ul>
  </section>
</template>
