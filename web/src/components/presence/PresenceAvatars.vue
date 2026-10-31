<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRealtimeStore } from '../../stores/realtime'
import { presenceColor } from './colors'

const props = defineProps<{ tableId: string; identity: string }>()
const realtime = useRealtimeStore()
const { t } = useI18n()
const collaborators = computed(() =>
  (realtime.presence[props.tableId] ?? []).filter(
    (user) => user.userId !== props.identity,
  ),
)

function initial(name: string) {
  return Array.from(name.trim())[0]?.toLocaleUpperCase() ?? ''
}
</script>

<template>
  <div
    v-if="collaborators.length"
    class="presence-avatars"
    role="list"
    :aria-label="t('presence.collaborators')"
  >
    <span
      v-for="user in collaborators"
      :key="user.userId"
      class="presence-avatar"
      role="listitem"
      :data-presence-user="user.userId"
      :aria-label="t('presence.viewing', { name: user.name })"
      :title="t('presence.viewing', { name: user.name })"
      :style="{ backgroundColor: presenceColor(user.userId) }"
    >
      {{ initial(user.name) }}
    </span>
  </div>
  <p v-if="realtime.error && realtime.status === 'connected'" role="alert">
    {{ t('presence.unavailable') }}
  </p>
</template>

<style scoped>
.presence-avatars {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  padding-inline-start: 4px;
}
.presence-avatar {
  display: grid;
  width: 32px;
  height: 32px;
  margin-inline-start: -4px;
  place-items: center;
  border: 2px solid var(--surface);
  border-radius: 50%;
  color: #fff;
  font-size: 13px;
  font-weight: 700;
}
</style>
