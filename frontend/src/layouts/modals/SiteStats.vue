<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="900"
  >
    <v-card
      class="rounded-lg"
      :loading="loading"
    >
      <v-card-title class="d-flex align-center">
        {{ $t('stats.sitesTitle') }}
        <v-spacer />
        <v-btn
          variant="tonal"
          color="success"
          density="comfortable"
          prepend-icon="mdi-microsoft-excel"
          class="me-2"
          :disabled="!sites.length"
          @click="exportExcel"
        >
          {{ $t('stats.exportExcel') }}
        </v-btn>
        <v-btn
          variant="text"
          density="comfortable"
          prepend-icon="mdi-download"
          class="me-2"
          :disabled="!sites.length"
          @click="exportCsv"
        >
          {{ $t('stats.exportCsv') }}
        </v-btn>
        <v-icon
          icon="mdi-refresh"
          class="me-2"
          :class="{ 'mdi-spin': loading }"
          @click="loadData"
        />
        <v-icon
          icon="mdi-close"
          @click="$emit('close')"
        />
      </v-card-title>
      <v-card-subtitle style="margin-top: -10px">
        {{ $t('objects.user') + " : " + user }}
      </v-card-subtitle>
      <v-card-text>
        <div class="text-center">
          <v-btn-toggle
            v-model="preset"
            density="compact"
            color="primary"
            border
            mandatory
            @update:model-value="selectPreset"
          >
            <v-btn
              v-for="p in presets"
              :key="p.value"
              :value="p.value"
            >
              {{ p.title }}
            </v-btn>
            <v-btn :value="0">
              <v-icon icon="mdi-calendar-range" />
            </v-btn>
          </v-btn-toggle>
        </div>
        <v-row
          v-if="preset === 0"
          dense
          justify="center"
          class="mt-2"
        >
          <v-col
            cols="12"
            sm="5"
          >
            <DatePick
              :expiry="start"
              :label="$t('stats.from')"
              input-id="sitesFrom"
              @submit="(v: number) => { start = v; loadData() }"
            />
          </v-col>
          <v-col
            cols="12"
            sm="5"
          >
            <DatePick
              :expiry="end"
              :label="$t('stats.to')"
              input-id="sitesTo"
              @submit="(v: number) => { end = v; loadData() }"
            />
          </v-col>
        </v-row>
        <v-row
          dense
          justify="center"
          class="mt-2"
        >
          <v-col cols="auto">
            <v-chip
              size="small"
              color="success"
              label
            >
              {{ $t('stats.download') }}: {{ fmt(totalDown) }}
            </v-chip>
          </v-col>
          <v-col cols="auto">
            <v-chip
              size="small"
              color="warning"
              label
            >
              {{ $t('stats.upload') }}: {{ fmt(totalUp) }}
            </v-chip>
          </v-col>
          <v-col cols="auto">
            <v-chip
              size="small"
              color="primary"
              label
            >
              {{ $t('stats.total') }}: {{ fmt(totalUp + totalDown) }}
            </v-chip>
          </v-col>
        </v-row>
        <v-alert
          v-if="missing.length"
          type="warning"
          variant="tonal"
          density="compact"
          class="mt-2"
          :text="$t('stats.sitesMissing', { nodes: missing.join('، ') })"
        />
        <v-text-field
          v-model="search"
          :label="$t('stats.search')"
          prepend-inner-icon="mdi-magnify"
          density="compact"
          hide-details
          clearable
          class="mt-3"
        />
        <v-data-table
          :headers="headers"
          :items="sites"
          :search="search"
          :loading="loading"
          density="compact"
          :items-per-page="25"
          :sort-by="[{ key: 'total', order: 'desc' }]"
        >
          <template #item.down="{ item }">
            <span dir="ltr">{{ fmt(item.down) }}</span>
          </template>
          <template #item.up="{ item }">
            <span dir="ltr">{{ fmt(item.up) }}</span>
          </template>
          <template #item.total="{ item }">
            <span dir="ltr">{{ fmt(item.total) }}</span>
          </template>
        </v-data-table>
        <div class="text-caption text-medium-emphasis mt-2">
          {{ $t('stats.sitesHint') }}
        </div>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { HumanReadable } from '@/plugins/utils'
import DatePick from '@/components/DateTime.vue'
import { siteRows, siteCsv, type SiteRow, type SiteReport } from './siteStats'
import { siteSheet, siteXlsx } from './siteXlsx'

export default {
  components: { DatePick },
  props: {
    visible: { type: Boolean, required: true },
    user: { type: String, required: true },
  },
  emits: ['close'],
  data() {
    return {
      loading: false,
      preset: 24,
      start: 0,
      end: 0,
      search: '',
      sites: <SiteRow[]>[],
      missing: <string[]>[],
      presets: [
        { value: 1, title: i18n.global.n(1) + i18n.global.t('date.h') },
        { value: 24, title: i18n.global.n(1) + i18n.global.t('date.d') },
        { value: 168, title: i18n.global.n(7) + i18n.global.t('date.d') },
        { value: 720, title: i18n.global.n(30) + i18n.global.t('date.d') },
      ],
    }
  },
  computed: {
    headers() {
      return [
        { title: i18n.global.t('stats.site'), key: 'site' },
        { title: i18n.global.t('stats.download'), key: 'down' },
        { title: i18n.global.t('stats.upload'), key: 'up' },
        { title: i18n.global.t('stats.total'), key: 'total' },
      ]
    },
    totalUp(): number { return this.sites.reduce((a, s) => a + s.up, 0) },
    totalDown(): number { return this.sites.reduce((a, s) => a + s.down, 0) },
  },
  watch: {
    visible(v: boolean) {
      if (v) {
        this.preset = 24
        this.search = ''
        this.selectPreset(24)
      } else {
        this.sites = []
        this.missing = []
      }
    },
  },
  methods: {
    selectPreset(v: number) {
      const now = Math.floor(Date.now() / 1000)
      if (v !== 0) {
        this.end = now
        this.start = now - v * 3600
      } else if (!this.start || !this.end) {
        this.end = now
        this.start = now - 86400
      }
      this.loadData()
    },
    async loadData() {
      if (!this.user || !this.start || !this.end || this.end <= this.start) return
      this.loading = true
      const data = await HttpUtils.get<SiteReport>('api/siteStats', { user: this.user, start: this.start, end: this.end })
      this.loading = false
      if (data.success && data.obj) {
        this.sites = siteRows(data.obj)
        this.missing = data.obj.missing ?? []
      } else {
        this.sites = []
        this.missing = []
      }
    },
    fmt(v: number) {
      return HumanReadable.sizeFormat(v)
    },
    fileName(ext: string) {
      const day = (t: number) => new Date(t * 1000).toISOString().slice(0, 10)
      return `${this.user}-sites-${day(this.start)}_${day(this.end)}.${ext}`
    },
    save(blob: Blob, name: string) {
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = name
      a.click()
      setTimeout(() => URL.revokeObjectURL(a.href), 1000)
    },
    // when is a window edge as people read it: local time with its offset,
    // and the Persian calendar first in Persian.
    when(t: number) {
      const d = new Date(t * 1000)
      const pad = (n: number) => String(n).padStart(2, '0')
      const off = -d.getTimezoneOffset()
      const zone = `UTC${off >= 0 ? '+' : '-'}${pad(Math.floor(Math.abs(off) / 60))}:${pad(Math.abs(off) % 60)}`
      const greg = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
      if (i18n.global.locale.value !== 'fa') return `${greg} (${zone})`
      try {
        const fa = new Intl.DateTimeFormat('fa-IR-u-ca-persian', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(d)
        return `${fa}  —  ${greg} (${zone})`
      } catch {
        return `${greg} (${zone})`
      }
    },
    exportExcel() {
      const t = (k: string) => i18n.global.t('stats.' + k)
      const sheet = siteSheet({
        title: t('xlsTitle'), user: t('xlsUser'), from: t('xlsFrom'), to: t('xlsTo'), sites: t('xlsSites'),
        rank: t('rank'), site: t('site'), download: t('download') + ' (MB)', upload: t('upload') + ' (MB)',
        total: t('total') + ' (MB)', share: t('share'), readable: t('readable'), sum: t('sum'),
      }, this.user, this.when(this.start), this.when(this.end), this.sites, v => HumanReadable.sizeFormat(v), i18n.global.locale.value === 'fa')
      const data = siteXlsx(this.user, sheet)
      this.save(new Blob([data.buffer as ArrayBuffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), this.fileName('xlsx'))
    },
    exportCsv() {
      const t = (k: string) => i18n.global.t('stats.' + k)
      const csv = siteCsv({ rank: t('rank'), site: t('site'), download: t('download'), upload: t('upload'), total: t('total'), share: t('share'), sum: t('sum') }, this.sites)
      // The byte-order mark makes Excel read the file as UTF-8.
      this.save(new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8' }), this.fileName('csv'))
    },
  },
}
</script>
