<template>
  <div class="d-flex align-center mb-3">
    <v-spacer />
    <v-btn
      icon="mdi-refresh"
      variant="tonal"
      size="small"
      :loading="loading"
      @click="load"
    />
  </div>
  <template v-if="report">
    <v-row class="mb-2">
      <v-col
        v-for="tile in tiles"
        :key="tile.label"
        cols="6"
        sm="4"
      >
        <v-card
          variant="tonal"
          rounded="lg"
          class="pa-3 text-center"
        >
          <div class="text-caption text-medium-emphasis">
            {{ tile.label }}
          </div>
          <div class="font-weight-bold">
            {{ tile.value }}
          </div>
          <div
            v-if="tile.sub"
            class="text-caption"
          >
            {{ tile.sub }}
          </div>
        </v-card>
      </v-col>
    </v-row>
    <div class="text-subtitle-2 mb-1">
      {{ $t('node.outages') }}
    </div>
    <v-table
      v-if="report.outages.length > 0"
      density="compact"
    >
      <thead>
        <tr>
          <th>{{ $t('node.outageStart') }}</th>
          <th>{{ $t('node.outageEnd') }}</th>
          <th>{{ $t('sessions.duration') }}</th>
          <th>{{ $t('node.filter.state') }}</th>
          <th>{{ $t('node.reason') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="o in report.outages"
          :key="o.id"
        >
          <td class="text-no-wrap">
            {{ fmtTime(o.start) }}
          </td>
          <td class="text-no-wrap">
            <v-chip
              v-if="!o.end"
              size="x-small"
              color="error"
            >
              {{ $t('node.ongoing') }}
            </v-chip>
            <span v-else>{{ fmtTime(o.end) }}</span>
          </td>
          <td class="text-no-wrap">
            {{ fmtDuration((o.end || now) - o.start) }}
          </td>
          <td>{{ $t(o.state === 'core-stopped' ? 'node.status.coreStopped' : 'node.status.offline') }}</td>
          <td
            class="node-reason"
            dir="auto"
          >
            {{ o.reason || '-' }}
          </td>
        </tr>
      </tbody>
    </v-table>
    <v-alert
      v-else
      type="success"
      variant="tonal"
      density="compact"
    >
      {{ $t('node.noOutages') }}
    </v-alert>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { fmtDuration, fmtTime, fmtUptime } from './format'
import type { NodeOutageReport } from '@/types/node'

const props = defineProps<{ nodeId: number; now: number }>()
const report = ref<NodeOutageReport | null>(null)
const loading = ref(false)

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<NodeOutageReport>('api/nodeOutages', { id: props.nodeId })
  loading.value = false
  if (msg.success && msg.obj) report.value = msg.obj
}
onMounted(load)

interface Tile {
  label: string
  value: string
  sub?: string
}

const tiles = computed((): Tile[] => {
  const r = report.value
  if (!r) return []
  const t = (k: string, v?: Record<string, unknown>) => (v ? i18n.global.t(k, v) : i18n.global.t(k))
  const down = (sec: number, count: number) => ({ value: fmtDuration(sec), sub: t('node.outageCount', { n: count }) })
  return [
    { label: `${t('node.uptime')} 24h`, value: fmtUptime(r.uptime24) },
    { label: `${t('node.uptime')} 7d`, value: fmtUptime(r.uptime7d) },
    { label: `${t('node.downtime')} 24h`, ...down(r.down24, r.count24) },
    { label: `${t('node.downtime')} 7d`, ...down(r.down7d, r.count7d) },
    { label: `${t('node.downtime')} 30d`, ...down(r.down30d, r.count30d) },
  ]
})
</script>

<style scoped>
.node-reason {
  overflow-wrap: anywhere;
  max-width: 320px;
}
</style>
