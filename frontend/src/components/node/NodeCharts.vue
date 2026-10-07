<template>
  <div class="d-flex align-center flex-wrap ga-2 mb-3">
    <v-btn-toggle
      v-model="hours"
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
  <v-alert
    v-if="loaded && empty"
    type="info"
    variant="tonal"
    density="compact"
  >
    {{ $t('noData') }}
  </v-alert>
  <v-row v-else-if="loaded">
    <v-col
      v-for="chart in charts"
      :key="chart.key + theme.global.name.value"
      cols="12"
      md="6"
    >
      <div class="text-subtitle-2 mb-1">
        {{ chart.title }}
      </div>
      <div class="node-chart">
        <Line
          :data="chart.data"
          :options="chart.options"
        />
      </div>
    </v-col>
  </v-row>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useTheme } from 'vuetify'
import { Line } from 'vue-chartjs'
import type { ChartData } from 'chart.js'
import HttpUtils from '@/plugins/httputil'
import { HumanReadable } from '@/plugins/utils'
import { i18n, locale } from '@/locales'
import { chartColors, lineOptions, type ChartLook } from './charts'
import { historySeries, type NodeHistory, type NodeHistoryPoint } from '@/types/node'

const props = defineProps<{ nodeId: number }>()
const theme = useTheme()
const hours = ref(6)
const loading = ref(false)
const loaded = ref(false)
const history = ref<NodeHistory | null>(null)
const loadedAt = ref(0)

const hourText = (n: number) => i18n.global.n(n) + i18n.global.t('date.h')
const dayText = (n: number) => i18n.global.n(n) + i18n.global.t('date.d')
const periods = [
  { value: 1, title: hourText(1) }, { value: 3, title: hourText(3) }, { value: 6, title: hourText(6) },
  { value: 12, title: hourText(12) }, { value: 24, title: dayText(1) }, { value: 72, title: dayText(3) }, { value: 168, title: dayText(7) },
]

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<NodeHistory>('api/nodeHistory', { id: props.nodeId, hours: hours.value })
  loading.value = false
  if (msg.success && msg.obj) {
    history.value = msg.obj
    loadedAt.value = Math.floor(Date.now() / 1000)
    loaded.value = true
  }
}
onMounted(load)

const series = computed(() => (history.value ? historySeries(history.value, loadedAt.value) : { times: [], points: [] }))
const empty = computed(() => !series.value.points.some(p => p !== null))

const look = computed((): ChartLook => ({
  text: theme.current.value.colors['on-surface'] as string,
  grid: theme.current.value.dark ? '#333333' : '#88888850',
}))

const labels = computed(() => series.value.times.map(t => {
  const d = new Date(t * 1000)
  return hours.value <= 24
    ? d.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })
    : d.toLocaleString(locale, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}))

// A value of a bucket the node answered in; the others are gaps.
const upValue = (p: NodeHistoryPoint | null, v: (p: NodeHistoryPoint) => number) => (p && p.uptime > 0 ? v(p) : null)

const dataset = (label: string, color: string, data: (number | null)[], fill = false) => ({
  label, data, borderColor: color, backgroundColor: color + '33', fill,
})

const charts = computed(() => {
  const pts = series.value.points
  const t = (k: string) => i18n.global.t(k)
  const pct = (v: number) => `${Math.round(v)}%`
  const bytes = (v: number) => (v > 0 ? HumanReadable.sizeFormat(v, 1) : '0')
  const make = (key: string, title: string, datasets: ChartData<'line'>['datasets'], format?: (v: number) => string, max?: number) => ({
    key, title, data: { labels: labels.value, datasets } as ChartData<'line'>, options: lineOptions(look.value, format, max),
  })
  return [
    make('load', t('node.chart.load'), [
      dataset('CPU', chartColors.cpu, pts.map(p => upValue(p, x => x.cpu))),
      dataset('RAM', chartColors.mem, pts.map(p => upValue(p, x => x.mem))),
      // Older nodes do not report their disk.
      dataset(t('node.disk'), chartColors.disk, pts.map(p => (p && p.uptime > 0 && p.disk > 0 ? p.disk : null))),
    ], pct, 100),
    make('latency', t('node.chart.latency'), [
      dataset(t('node.latency'), chartColors.latency, pts.map(p => upValue(p, x => x.latency))),
    ], v => `${Math.round(v)} ms`),
    make('users', t('node.chart.users'), [
      dataset(t('node.users'), chartColors.users, pts.map(p => upValue(p, x => x.online)), true),
    ]),
    make('net', t('node.chart.net'), [
      dataset(t('node.sent'), chartColors.up, pts.map(p => (p ? p.sent : null))),
      dataset(t('node.received'), chartColors.down, pts.map(p => (p ? p.recv : null))),
    ], bytes),
    make('uptime', t('node.chart.uptime'), [
      dataset(t('node.uptime'), chartColors.uptime, pts.map(p => (p && p.uptime >= 0 ? p.uptime : null)), true),
    ], pct, 100),
  ]
})
</script>

<style scoped>
.node-chart {
  position: relative;
  height: 220px;
}
</style>
