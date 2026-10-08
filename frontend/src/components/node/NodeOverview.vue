<template>
  <v-row>
    <v-col
      cols="12"
      md="6"
    >
      <div class="text-subtitle-2 mb-1">
        {{ $t('node.server') }}
      </div>
      <v-table density="compact">
        <tbody>
          <tr
            v-for="row in serverRows"
            :key="row.label"
          >
            <td class="text-medium-emphasis">
              {{ row.label }}
            </td>
            <td
              dir="ltr"
              class="text-end node-value"
            >
              {{ row.value }}
            </td>
          </tr>
        </tbody>
      </v-table>
    </v-col>
    <v-col
      cols="12"
      md="6"
    >
      <div class="text-subtitle-2 mb-1">
        {{ $t('node.settings') }}
      </div>
      <v-table density="compact">
        <tbody>
          <tr
            v-for="row in settingRows"
            :key="row.label"
          >
            <td class="text-medium-emphasis">
              {{ row.label }}
            </td>
            <td class="text-end node-value">
              {{ row.value }}
            </td>
          </tr>
        </tbody>
      </v-table>
      <v-alert
        v-if="warnings.length > 0"
        type="warning"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        <div
          v-for="w in warnings"
          :key="w"
        >
          {{ w }}
        </div>
      </v-alert>
      <v-alert
        v-if="status?.error"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        {{ status.error }}
      </v-alert>
    </v-col>
  </v-row>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { i18n } from '@/locales'
import { fmtBytes, fmtDate, fmtDuration, fmtPercent, fmtTime } from './format'
import { warningText } from './warnings'
import { alertDefaults, nodeVersion, usage, type AlertKey, type Node, type NodeStatus } from '@/types/node'
import Data from '@/store/modules/data'

const props = defineProps<{ node: Node; status?: NodeStatus; now: number }>()
const t = (key: string, values?: Record<string, unknown>) => (values ? i18n.global.t(key, values) : i18n.global.t(key))

const warnings = computed(() => (props.node.enable ? (props.status?.warnings ?? []).map(warningText) : []))

const serverRows = computed(() => {
  const s = props.status
  const mem = (m?: { current: number; total: number }) => (m && m.total > 0 ? `${fmtBytes(m.current)} / ${fmtBytes(m.total)} (${fmtPercent(usage(m))})` : '-')
  const rows = [
    { label: t('node.hostName'), value: s?.hostName || '-' },
    { label: 'CPU', value: s?.cpuCount ? `${s.cpuCount} × ${s.cpuType || '-'}` : (s?.cpuType || '-') },
    { label: 'IPv4', value: (s?.ipv4 ?? []).join(', ') || '-' },
    { label: 'IPv6', value: (s?.ipv6 ?? []).join(', ') || '-' },
    { label: 'RAM', value: mem(s?.mem) },
    { label: t('node.disk'), value: mem(s?.disk) },
    { label: 'Swap', value: mem(s?.swap) },
    { label: t('node.bootTime'), value: s?.bootTime ? `${fmtTime(s.bootTime)} (${fmtDuration(props.now - s.bootTime)})` : '-' },
    { label: t('node.coreUptime'), value: s?.coreUptime ? fmtDuration(s.coreUptime) : '-' },
    { label: t('node.panelVersion'), value: nodeVersion(s) || '-' },
    { label: t('node.coreVersion'), value: s?.coreVersion || '-' },
    { label: t('node.certExpiry'), value: s?.certExpiry ? `${fmtDate(s.certExpiry)} (${t('node.daysLeft', { n: Math.floor((s.certExpiry - props.now) / 86400) })})` : '-' },
    { label: t('node.checkedAt'), value: fmtTime(s?.checkedAt) },
    { label: t('node.lastSeen'), value: fmtTime(s?.lastOnline || props.node.lastSeen) },
  ]
  if (s?.downSince) rows.push({ label: t('node.downSince'), value: `${fmtTime(s.downSince)} (${fmtDuration(props.now - s.downSince)})` })
  return rows
})

const settingRows = computed(() => {
  const n = props.node
  const alert = (key: AlertKey, unit: string) => {
    const v = n.alerts?.[key] ?? alertDefaults[key]
    return v > 0 ? `${v}${unit}` : t('node.alert.off')
  }
  const access = n.access ?? { groups: [], clients: [] }
  const clientNames = access.clients.map(id => Data().clients.find(c => c.id === id)?.name ?? `#${id}`)
  const cap = n.cap
  const rows = [
    { label: t('node.tags'), value: (n.tags ?? []).map(x => '#' + x).join(' ') || '-' },
    { label: t('node.country'), value: n.country || '-' },
    { label: t('node.sortOrder'), value: String(n.sortOrder ?? 0) },
    { label: t('node.inboundsClients'), value: `${n.inboundCount ?? 0} · ${n.clientCount ?? 0}` },
    {
      label: t('node.tab.alerts'),
      value: `CPU ${alert('cpu', '%')} · RAM ${alert('mem', '%')} · ${t('node.disk')} ${alert('disk', '%')} · Ping ${alert('ping', ' ms')} · ${t('node.alert.certShort')} ${alert('certDays', 'd')}`,
    },
    {
      label: t('node.cap.title'),
      value: cap && cap.limit > 0
        ? `${fmtBytes(cap.limit)} · ${t('node.cap.mode.' + (cap.mode || 'total'))} · ${t('node.cap.dayShort', { day: cap.day || 1 })}${cap.hide ? ' · ' + t('node.cap.hideShort') : ''}`
        : t('node.cap.none'),
    },
    { label: t('node.hideDownShort'), value: n.hideDown ? t('enable') : t('disable') },
    {
      label: t('node.tab.access'),
      value: access.groups.length + access.clients.length === 0
        ? t('node.access.everyone')
        : [...access.groups.map(g => `${t('node.access.group')}: ${g}`), ...clientNames].join(', '),
    },
  ]
  if (n.desc) rows.push({ label: t('node.desc'), value: n.desc })
  return rows
})
</script>

<style scoped>
.node-value {
  overflow-wrap: anywhere;
}
</style>
