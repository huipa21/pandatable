<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { syntheticSource } from '../spikes/synthetic'
import type { WindowStats } from '../components/grid/window'

const DataGrid = defineAsyncComponent(
  () => import('../components/grid/SpikeDataGrid.vue'),
)
const { t } = useI18n()
const running = ref(false)
const grouped = ref(false)
const stats = ref<WindowStats>()
const grid = ref<{ jump(index: number): Promise<void> }>()
const media = window.matchMedia('(min-width: 768px)')
const desktop = ref(media.matches)
const resized = (event: MediaQueryListEvent) => {
  desktop.value = event.matches
}
media.addEventListener('change', resized)
onBeforeUnmount(() => media.removeEventListener('change', resized))
const provider = computed(() =>
  syntheticSource(
    grouped.value ? 100 : 200_000,
    (id) => t('spike.row', { id }),
    (index) => (index ? t('spike.groupB') : t('spike.groupA')),
  ),
)
const requestStatus = computed(() => ({
  loaded: t('spike.requestLoaded'),
  pending: t('spike.requestPending'),
  cancelled: t('spike.requestCancelled'),
  stale: t('spike.requestStale'),
  failed: t('spike.requestFailed'),
}))
</script>

<template>
  <section class="grid-spike">
    <p class="eyebrow">{{ t('spike.eyebrow') }}</p>
    <h1>{{ t('spike.title') }}</h1>
    <p>{{ t('spike.disclaimer') }}</p>
    <RouterLink to="/login">{{ t('common.backToLogin') }}</RouterLink>
    <p v-if="!desktop" role="status">{{ t('spike.mobile') }}</p>
    <template v-else>
      <div class="spike-controls">
        <button :aria-pressed="running" @click="running = !running">
          {{ running ? t('spike.stop') : t('spike.start') }}
        </button>
        <label>
          <input v-model="grouped" type="checkbox" />
          {{ t('spike.groupSample') }}
        </label>
        <button :disabled="!running || grouped" @click="grid?.jump(0)">
          {{ t('spike.begin') }}
        </button>
        <button :disabled="!running || grouped" @click="grid?.jump(100000)">
          {{ t('spike.middle') }}
        </button>
        <button :disabled="!running || grouped" @click="grid?.jump(199999)">
          {{ t('spike.end') }}
        </button>
      </div>
      <p>{{ grouped ? t('spike.groupNotice') : t('spike.instructions') }}</p>
      <DataGrid
        v-if="running"
        :key="String(grouped)"
        ref="grid"
        :provider="provider"
        :grouped="grouped"
        @stats="stats = $event"
      />
      <section
        v-if="stats"
        :aria-label="t('spike.telemetry')"
        class="spike-telemetry"
      >
        <h2>{{ t('spike.telemetry') }}</h2>
        <p data-testid="window-stats">
          {{
            t('spike.stats', {
              total: stats.total,
              page: stats.activePage,
              rows: stats.cachedRows,
              pending: stats.pending,
              edits: stats.editedCells,
            })
          }}
        </p>
        <output data-testid="window-json" class="spike-json">{{
          JSON.stringify(stats)
        }}</output>
        <table>
          <thead>
            <tr>
              <th>{{ t('spike.requestRange') }}</th>
              <th>{{ t('spike.requestLatency') }}</th>
              <th>{{ t('spike.requestStatus') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="request in stats.requests" :key="request.id">
              <td>
                {{
                  t('spike.range', {
                    start: request.offset,
                    end: request.offset + request.limit - 1,
                  })
                }}
              </td>
              <td>{{ request.ms }}</td>
              <td>
                {{ requestStatus[request.status] }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
  </section>
</template>

<style scoped>
.spike-controls {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  align-items: center;
  margin-top: 16px;
}
button {
  padding: 8px 12px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
}
button:disabled {
  opacity: 0.5;
  cursor: default;
}
.spike-json {
  display: none;
}
.spike-telemetry {
  font-size: 12px;
}
th,
td {
  text-align: left;
  padding: 2px 16px 2px 0;
}
</style>
