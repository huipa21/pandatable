<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DirectGrant, GrantLevel } from '../../api/types'
import { accessLevelLabel } from '../../i18n'
import type { ShareChange } from '../../stores/sharing'

const props = defineProps<{ grant: DirectGrant; disabled: boolean }>()
const emit = defineEmits<{ review: [change: ShareChange] }>()
const { t } = useI18n()
const level = ref<GrantLevel>(props.grant.level)
watch(
  () => props.grant.level,
  (value) => {
    level.value = value
  },
)
const name = computed(
  () =>
    props.grant.principal?.name ??
    t('sharing.missingPrincipal', { id: props.grant.principalId }),
)
const rank = { read: 1, write: 2, owner: 3 }
const canReplace = computed(
  () => props.grant.principal && props.grant.principal.status !== 'disabled',
)
const inheritedHigher = computed(
  () =>
    props.grant.inheritedLevel &&
    rank[props.grant.inheritedLevel] > rank[level.value],
)
function review(remove = false) {
  emit('review', {
    name: name.value,
    principalType: props.grant.principalType,
    principalId: props.grant.principalId,
    level: level.value,
    removeId: remove ? props.grant.id : undefined,
    inheritedLevel: props.grant.inheritedLevel,
  })
}
</script>

<template>
  <li class="share-grant" :data-principal-id="grant.principalId">
    <p>
      <strong>{{ name }}</strong>
      <span v-if="grant.principal?.username">{{
        t('sharing.username', { username: grant.principal.username })
      }}</span>
      <span>{{
        grant.principalType === 'user' ? t('sharing.user') : t('sharing.group')
      }}</span>
      <span v-if="grant.principal?.status === 'disabled'">{{
        t('sharing.disabled')
      }}</span>
    </p>
    <p>
      {{ t('sharing.directLevel', { level: accessLevelLabel(grant.level) }) }}
    </p>
    <p v-if="grant.inheritedLevel">
      {{
        t('sharing.inheritedLevel', {
          level: accessLevelLabel(grant.inheritedLevel),
        })
      }}
    </p>
    <label>
      <span>{{ t('sharing.levelFor', { name }) }}</span>
      <select
        v-model="level"
        :aria-label="t('sharing.levelFor', { name })"
        :disabled="disabled || !canReplace"
      >
        <option value="read">{{ t('nav.level.read') }}</option>
        <option value="write">{{ t('nav.level.write') }}</option>
        <option value="owner">{{ t('nav.level.owner') }}</option>
      </select>
    </label>
    <p v-if="inheritedHigher" class="share-warning">
      {{
        t('sharing.inheritedHigher', {
          level: accessLevelLabel(grant.inheritedLevel!),
        })
      }}
    </p>
    <div class="nav-actions">
      <button
        :disabled="disabled || !canReplace || level === grant.level"
        @click="review()"
      >
        {{ t('sharing.change') }}
      </button>
      <button :disabled="disabled" @click="review(true)">
        {{ t('sharing.remove') }}
      </button>
    </div>
  </li>
</template>
