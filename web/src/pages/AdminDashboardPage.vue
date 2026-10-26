<script setup lang="ts">
import { computed, watch } from 'vue'
import { NAlert, NButton } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { errorMessage, RequestError } from '../api/client'
import type { SystemMetric } from '../api/types'
import { useSystemMetrics } from '../composables/useSystemMetrics'
import { useAuthStore } from '../stores/auth'

const { t, locale } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const { snapshot, loading, error, refresh } = useSystemMetrics()

watch(error, async (cause) => {
  if (!(cause instanceof RequestError)) return
  if (cause.code === 'ERR_UNAUTHENTICATED') {
    auth.setUser(null)
    await router.replace('/login')
  } else if (cause.code === 'ERR_PASSWORD_CHANGE_REQUIRED') {
    await router.replace('/change-password')
  } else if (cause.code === 'ERR_PERMISSION_DENIED') {
    await router.replace('/home')
  }
})

function number(value: number) {
  return new Intl.NumberFormat(locale.value, {
    maximumFractionDigits: 2,
  }).format(value)
}
function bytes(value: number) {
  const units = [
    t('dashboard.units.bytes'),
    t('dashboard.units.kib'),
    t('dashboard.units.mib'),
    t('dashboard.units.gib'),
    t('dashboard.units.tib'),
  ]
  const index =
    value > 0
      ? Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
      : 0
  return t('dashboard.quantity', {
    value: number(value / 1024 ** index),
    unit: units[index],
  })
}
function time(value: string) {
  return new Intl.DateTimeFormat(locale.value, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value))
}
function reason(value: SystemMetric<unknown>['reason']) {
  const labels = {
    initializing: t('dashboard.reasons.initializing'),
    unsupported: t('dashboard.reasons.unsupported'),
    notFound: t('dashboard.reasons.notFound'),
    permission: t('dashboard.reasons.permission'),
    io: t('dashboard.reasons.io'),
    limit: t('dashboard.reasons.limit'),
    timeout: t('dashboard.reasons.timeout'),
    invalid: t('dashboard.reasons.invalid'),
    query: t('dashboard.reasons.query'),
  }
  return value
    ? (labels[value] ?? t('dashboard.unavailable'))
    : t('dashboard.unavailable')
}

function row<T>(
  id: string,
  label: string,
  metric: SystemMetric<T>,
  format: (value: T) => string,
  timestamp = false,
) {
  return {
    id,
    label,
    text:
      metric.status === 'ready' && metric.value !== null
        ? format(metric.value)
        : reason(metric.reason),
    available: metric.status === 'ready' && metric.value !== null,
    at: timestamp ? metric.collectedAt : undefined,
  }
}

const cards = computed(() => {
  const s = snapshot.value
  if (!s) return []
  return [
    {
      title: t('dashboard.cpu'),
      rows: [
        row('cpu', t('dashboard.cpuPercent'), s.cpuPercent, (v) =>
          t('dashboard.percent', { value: number(v) }),
        ),
        row('load1', t('dashboard.load1'), s.load, (v) => number(v.one)),
        row('load5', t('dashboard.load5'), s.load, (v) => number(v.five)),
        row('load15', t('dashboard.load15'), s.load, (v) => number(v.fifteen)),
      ],
    },
    {
      title: t('dashboard.memory'),
      rows: [
        row('memoryUsed', t('dashboard.used'), s.memory, (v) =>
          bytes(v.usedBytes),
        ),
        row('memoryTotal', t('dashboard.total'), s.memory, (v) =>
          bytes(v.totalBytes),
        ),
        row('memoryAvailable', t('dashboard.available'), s.memory, (v) =>
          bytes(v.availableBytes),
        ),
        row('rss', t('dashboard.rss'), s.appRssBytes, bytes),
      ],
    },
    {
      title: t('dashboard.container'),
      rows: [
        row('containerCurrent', t('dashboard.current'), s.container, (v) =>
          bytes(v.currentBytes),
        ),
        row('containerLimit', t('dashboard.limit'), s.container, (v) =>
          v.unlimited
            ? t('dashboard.unlimited')
            : v.limitBytes === null
              ? t('dashboard.unavailable')
              : bytes(v.limitBytes),
        ),
      ],
    },
    {
      title: t('dashboard.disk'),
      rows: [
        row('diskTotal', t('dashboard.total'), s.disk, (v) =>
          bytes(v.totalBytes),
        ),
        row('diskUsed', t('dashboard.used'), s.disk, (v) => bytes(v.usedBytes)),
        row('diskFree', t('dashboard.free'), s.disk, (v) => bytes(v.freeBytes)),
      ],
    },
    {
      title: t('dashboard.database'),
      rows: [
        row('dbSize', t('dashboard.dbSize'), s.database, (v) =>
          bytes(v.sizeBytes),
        ),
        row('connections', t('dashboard.connections'), s.database, (v) =>
          number(v.connections),
        ),
        row(
          'activeConnections',
          t('dashboard.activeConnections'),
          s.database,
          (v) => number(v.activeConnections),
        ),
        row('sharedBuffers', t('dashboard.sharedBuffers'), s.database, (v) =>
          bytes(v.sharedBuffersBytes),
        ),
      ],
    },
    {
      title: t('dashboard.storage'),
      rows: [
        row('files', t('dashboard.files'), s.filesBytes, bytes, true),
        row('backups', t('dashboard.backups'), s.backupsBytes, bytes, true),
      ],
    },
    {
      title: t('dashboard.versions'),
      rows: [
        {
          id: 'appVersion',
          label: t('dashboard.appVersion'),
          text: s.versions.app,
          available: true,
        },
        {
          id: 'goVersion',
          label: t('dashboard.goVersion'),
          text: s.versions.go,
          available: true,
        },
        row(
          'pgVersion',
          t('dashboard.pgVersion'),
          s.versions.postgres,
          (v) => v,
        ),
        row(
          'migration',
          t('dashboard.migration'),
          s.versions.migration,
          number,
        ),
        {
          id: 'uptime',
          label: t('dashboard.uptime'),
          text: t('dashboard.seconds', { value: number(s.uptimeSeconds) }),
          available: true,
        },
      ],
    },
  ]
})
</script>

<template>
  <section class="dashboard" :aria-busy="loading">
    <div class="dashboard-heading">
      <h1>{{ t('dashboard.title') }}</h1>
      <NButton :loading="loading" @click="refresh">
        {{ t('dashboard.refresh') }}
      </NButton>
    </div>
    <p>{{ t('dashboard.scope') }}</p>
    <p>{{ t('dashboard.policy') }}</p>
    <p v-if="loading && !snapshot" role="status">
      {{ t('dashboard.loading') }}
    </p>
    <NAlert v-if="error" type="error" role="alert">
      {{ errorMessage(error) }}
      <p v-if="snapshot">{{ t('dashboard.stale') }}</p>
      <NButton @click="refresh">{{ t('auth.retry') }}</NButton>
    </NAlert>
    <template v-if="snapshot">
      <p
        class="dashboard-updated"
        role="status"
        :data-collected-at="snapshot.collectedAt"
      >
        {{ t('dashboard.updated', { time: time(snapshot.collectedAt) }) }}
      </p>
      <div class="dashboard-grid">
        <section v-for="card in cards" :key="card.title" class="dashboard-card">
          <h2>{{ card.title }}</h2>
          <dl>
            <div
              v-for="metric in card.rows"
              :key="metric.id"
              :data-metric="metric.id"
              :data-available="metric.available"
            >
              <dt>{{ metric.label }}</dt>
              <dd>{{ metric.text }}</dd>
              <small v-if="'at' in metric && metric.at">
                {{ t('dashboard.observed', { time: time(metric.at) }) }}
              </small>
            </div>
          </dl>
        </section>
      </div>
    </template>
  </section>
</template>

<style scoped>
.dashboard {
  max-width: 1120px;
  margin: 0 auto;
}
.dashboard-heading {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 290px), 1fr));
  gap: 16px;
}
.dashboard-card {
  min-width: 0;
  border: 1px solid #8b969640;
  border-radius: 12px;
  padding: 20px;
}
h2 {
  margin-top: 0;
  font-size: 1.15rem;
}
dl {
  margin: 0;
}
dl > div {
  margin-top: 14px;
}
dt {
  opacity: 0.75;
}
dd {
  margin: 3px 0 0;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  overflow-wrap: anywhere;
}
small {
  display: block;
  margin-top: 4px;
  opacity: 0.75;
}
</style>
