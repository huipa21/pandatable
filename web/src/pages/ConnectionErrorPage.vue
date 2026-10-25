<script setup lang="ts">
import { NAlert, NButton } from 'naive-ui'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <section class="welcome">
    <h1>{{ t('auth.connectionError') }}</h1>
    <NAlert type="error" role="alert">{{ t('errors.ERR_INTERNAL') }}</NAlert>
    <RouterLink to="/login">
      <NButton>{{ t('auth.retry') }}</NButton>
    </RouterLink>
  </section>
</template>
