<template>
  <div class="d-flex align-center flex-wrap ga-2 mb-3">
    <v-btn-toggle
      v-model="period"
      mandatory
      density="compact"
      variant="outlined"
      divided
      @update:model-value="load"
    >
      <v-btn
        v-for="p in periods"
        :key="p.value"
        :value="p.value"
        size="small"
      >
        {{ p.title }}
      </v-btn>
    </v-btn-toggle>
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
    <div class="d-flex flex-wrap ga-2 mb-3">
      <v-chip
        v-for="c in summaryChips"
        :key="c.label"
        label
        variant="tonal"
      >
        {{ c.label }}:&nbsp;<span dir="ltr">↑ {{ fmtBytes(c.up) }} · ↓ {{ fmtBytes(c.down) }}</span>
      </v-chip>
      <v-chip
        v-if="cap !== null"
        label
        variant="tonal"
        :color="loadColor(cap)"
      >
        {{ $t('node.cap.title') }}:&nbsp;<span dir="ltr">{{ fmtBytes(report.summary.capUsed) }} / {{ fmtBytes(report.summary.capLimit) }} ({{ fmtPercent(cap) }})</span>
      </v-chip>
    </div>
    <div class="text-subtitle-2 mb-1">
      {{ $t('node.trafficOf', { total: fmtBytes(periodUp + periodDown) }) }}
    </div>
    <div class="node-chart mb-4">
      <Bar
        :key="theme.global.name.value"
        :data="chartData"
        :options="options"
      />
    </div>
    <div class="text-subtitle-2 mb-1">
      {{ $t('node.topClients') }}
    </div>
    <v-alert
      v-if="report.clientsNote"
      type="info"
      variant="tonal"
      density="compact"
    >
      {{ $t('node.clientsNote.' + report.clientsNote) }}
    </v-alert>
    <v-table
      v-else-if="report.clients.length > 0"
      density="compact"
    >
      <thead>
        <tr>
          <th>{{ $t('client.name') }}</th>
          <th class="text-end">
            {{ $t('stats.upload') }}
          </th>
          <th class="text-end">
            {{ $t('stats.download') }}
          </th>
          <th class="text-end">
            {{ $t('node.total') }}
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="c in report.clients"
          :key="c.name"
        >
          <td dir="auto">
            {{ c.name }}
          </td>
          <td
            class="text-end"
            dir="ltr"
          >
            {{ fmtBytes(c.up) }}
          </td>
          <td
            class="text-end"
            dir="ltr"
          >
            {{ fmtBytes(c.down) }}
          </td>
          <td
            class="text-end"
            dir="ltr"
          >
            {{ fmtBytes(c.up + c.down) }}
          </td>
        </tr>
      </tbody>
    </v-table>
    <div
      v-else
      class="text-medium-emphasis text-caption"
    >
      {{ $t('noData') }}
    </div>
    <div class="text-caption text-medium-emphasis mt-2">
      {{ $t('node.trafficHint') }}
    </div>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useTheme } from 'vuetify'
import { Bar } from 'vue-chartjs'
import type { ChartData } from 'chart.js'
import HttpUtils from '@/plugins/httputil'
import { i18n, locale } from '@/locales'
import { barOptions, chartColors } from './charts'
import { fmtBytes, fmtPercent, loadColor } from './format'
import { capPercent, type NodeTrafficReport } from '@/types/node'

const props = defineProps<{ nodeId: number }>()
const theme = useTheme()
const period = ref('24h')
const loading = ref(false)
const report = ref<NodeTrafficReport | null>(null)

const periods = [
  { value: '24h', title: i18n.global.n(24) + i18n.global.t('date.h') },
  { value: '7d', title: i18n.global.n(7) + i18n.global.t('date.d') },
  { value: '30d', title: i18n.global.n(30) + i18n.global.t('date.d') },
  { value: '12m', title: i18n.global.t('node.months12') },
]

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<NodeTrafficReport>('api/nodeTraffic', { id: props.nodeId, period: period.value })
  loading.value = false
  if (msg.success && msg.obj) report.value = msg.obj
}
onMounted(load)

const cap = computed(() => capPercent(report.value?.summary))
const periodUp = computed(() => (report.value?.points ?? []).reduce((a, p) => a + p.up, 0))
const periodDown = computed(() => (report.value?.points ?? []).reduce((a, p) => a + p.down, 0))
const summaryChips = computed(() => {
  const s = report.value?.summary
  if (!s) return []
  return [
    { label: i18n.global.t('node.trafficToday'), up: s.todayUp, down: s.todayDown },
    { label: i18n.global.t('node.trafficMonth'), up: s.monthUp, down: s.monthDown },
    { label: i18n.global.t('node.total'), up: s.totalUp, down: s.totalDown },
  ]
})

const label = (t: number) => {
  const d = new Date(t * 1000)
  switch (report.value?.period) {
    case '24h': return d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })
    case '12m': return d.toLocaleDateString(locale, { year: 'numeric', month: 'short' })
    default: return d.toLocaleDateString(locale, { month: '2-digit', day: '2-digit' })
  }
}

const chartData = computed((): ChartData<'bar'> => {
  const points = report.value?.points ?? []
  return {
    labels: points.map(p => label(p.t)),
    datasets: [
      { label: i18n.global.t('node.sent'), data: points.map(p => p.up), backgroundColor: chartColors.up },
      { label: i18n.global.t('node.received'), data: points.map(p => p.down), backgroundColor: chartColors.down },
    ],
  }
})
const options = computed(() => barOptions({
  text: theme.current.value.colors['on-surface'] as string,
  grid: theme.current.value.dark ? '#333333' : '#88888850',
}, v => fmtBytes(v)))
</script>

<style scoped>
.node-chart {
  position: relative;
  height: 260px;
}
</style>
