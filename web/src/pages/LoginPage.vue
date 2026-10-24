<script setup lang="ts">
import { NCard, NTag } from 'naive-ui'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <section class="welcome">
    <p class="eyebrow">{{ t('login.eyebrow') }}</p>
    <h1>{{ t('login.title') }}</h1>
    <p class="welcome-description">{{ t('login.description') }}</p>
    <NCard class="welcome-card">
      <NTag type="success" round>{{ t('login.status') }}</NTag>
      <p>{{ t('login.notice') }}</p>
    </NCard>
  </section>
</template>
