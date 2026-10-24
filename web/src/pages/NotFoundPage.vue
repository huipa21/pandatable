<script setup lang="ts">
import { NButton } from 'naive-ui'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <section class="welcome">
    <h1>{{ t('nav.notFoundTitle') }}</h1>
    <p class="welcome-description">{{ t('nav.notFoundDescription') }}</p>
    <RouterLink to="/login">
      <NButton type="primary">{{ t('common.backToLogin') }}</NButton>
    </RouterLink>
  </section>
</template>
