<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'

const { t } = useI18n()
const auth = useAuthStore()
</script>

<template>
  <section class="welcome">
    <h1>{{ t('auth.homeTitle') }}</h1>
    <p>{{ t('setup.signedInAs', { name: auth.user?.displayName }) }}</p>
    <p class="welcome-description">{{ t('auth.homeNotice') }}</p>
  </section>
</template>
