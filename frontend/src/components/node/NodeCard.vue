<template>
  <v-card
    rounded="xl"
    elevation="4"
    :class="['h-100', 'd-flex', 'flex-column', { 'node-selected': selected }]"
  >
    <v-card-title class="d-flex align-center ga-2">
      <v-checkbox-btn
        v-if="selectable"
        :model-value="selected"
        density="compact"
        class="flex-grow-0"
        @update:model-value="(v: unknown) => emit('select', !!v)"
      />
      <span
        v-if="flag"
        :title="node.country"
      >{{ flag }}</span>
      <v-icon
        v-else
        icon="mdi-server-network"
      />
      <span
        class="text-truncate node-name"
        @click="emit('action', 'details')"
      >{{ node.name }}</span>
      <v-spacer />
      <v-chip
        size="small"
        label
        :color="viewColor[view]"
        :prepend-icon="viewIcon[view]"
      >
        {{ $t(viewLabelKey(view)) }}
      </v-chip>
    </v-card-title>
    <v-card-subtitle
      class="text-truncate"
      dir="ltr"
    >
      {{ node.baseUrl }}{{ node.webPath }}
    </v-card-subtitle>
    <v-card-text class="flex-grow-1">
      <div
        v-if="hasChips"
        class="d-flex flex-wrap ga-1 mb-3"
      >
        <v-chip
          v-for="t in node.tags ?? []"
          :key="t"
          size="x-small"
          variant="tonal"
          color="primary"
        >
          #{{ t }}
        </v-chip>
        <v-chip
          v-if="status?.hidden"
          size="x-small"
          color="warning"
          prepend-icon="mdi-eye-off"
        >
          {{ $t('node.hidden.' + status.hidden) }}
        </v-chip>
        <v-chip
          v-if="status?.filtered && status?.hidden != 'filtered'"
          size="x-small"
          color="error"
          prepend-icon="mdi-cancel"
        >
          {{ $t('node.filtered') }}
        </v-chip>
        <v-chip
          v-if="node.dirty"
          size="x-small"
          color="warning"
          prepend-icon="mdi-sync-alert"
        >
          {{ $t('node.syncDirty') }}
        </v-chip>
        <v-chip
          v-if="restricted"
          size="x-small"
          prepend-icon="mdi-account-lock"
        >
          {{ $t('node.access.restricted') }}
        </v-chip>
        <v-chip
          v-if="node.hideDown"
          size="x-small"
          prepend-icon="mdi-eye-off-outline"
          variant="outlined"
        >
          {{ $t('node.hideDownShort') }}
        </v-chip>
      </div>

      <div
        v-for="bar in bars"
        :key="bar.label"
        class="d-flex align-center ga-2 mb-1"
      >
        <span class="node-bar-label text-medium-emphasis">{{ bar.label }}</span>
        <v-progress-linear
          :model-value="bar.value ?? 0"
          :color="loadColor(bar.value)"
          height="8"
          rounded
          class="flex-grow-1"
        />
        <span class="node-bar-value">{{ fmtPercent(bar.value) }}</span>
        <v-tooltip
          v-if="bar.hint"
          activator="parent"
          location="top"
          :text="bar.hint"
        />
      </div>

      <v-row
        dense
        class="mt-2"
      >
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.latency') }}
          </div>
          <strong>{{ live ? `${live.latency} ms` : '-' }}</strong>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.users') }}
          </div>
          <a
            v-if="live"
            href="#"
            class="text-decoration-none font-weight-bold"
            @click.prevent="emit('action', 'online')"
          >
            <v-icon
              icon="mdi-account-multiple"
              size="small"
            /> {{ live.online ?? 0 }}
          </a>
          <strong v-else>-</strong>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.speed') }}
          </div>
          <span
            v-if="live"
            dir="ltr"
          >↑ {{ fmtSpeed(live.netUp) }} · ↓ {{ fmtSpeed(live.netDown) }}</span>
          <span v-else>-</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.uptime') }} (24h · 7d)
          </div>
          <span>{{ fmtUptime(status?.uptime24) }} · {{ fmtUptime(status?.uptime7d) }}</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.trafficToday') }}
          </div>
          <span dir="ltr">↑ {{ fmtBytes(traffic?.todayUp) }} · ↓ {{ fmtBytes(traffic?.todayDown) }}</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.trafficMonth') }}
          </div>
          <span dir="ltr">↑ {{ fmtBytes(traffic?.monthUp) }} · ↓ {{ fmtBytes(traffic?.monthDown) }}</span>
        </v-col>
        <v-col
          v-if="cap !== null"
          cols="12"
        >
          <div class="d-flex text-caption">
            <span class="text-medium-emphasis">{{ $t('node.cap.title') }}</span>
            <v-spacer />
            <span dir="ltr">{{ fmtBytes(traffic?.capUsed) }} / {{ fmtBytes(traffic?.capLimit) }} ({{ fmtPercent(cap) }})</span>
          </div>
          <v-progress-linear
            :model-value="Math.min(cap, 100)"
            :color="loadColor(cap)"
            height="8"
            rounded
          />
          <div
            v-if="traffic?.capEnd"
            class="text-caption text-medium-emphasis"
          >
            {{ $t('node.cap.resets', { date: fmtDate(traffic.capEnd) }) }}
          </div>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.panelVersion') }} · {{ $t('node.coreVersion') }}
          </div>
          <span dir="ltr">{{ status?.appFull || status?.appVersion || '-' }} · {{ status?.coreVersion || '-' }}</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.inboundsClients') }}
          </div>
          <span>{{ node.inboundCount ?? 0 }} · {{ node.clientCount ?? 0 }}</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.lastSeen') }}
          </div>
          <span>{{ fmtTime(status?.lastOnline || node.lastSeen) }}</span>
        </v-col>
        <v-col cols="6">
          <div class="text-medium-emphasis text-caption">
            {{ $t('node.lastSync') }}
          </div>
          <span>{{ fmtTime(node.lastSync) }}</span>
        </v-col>
      </v-row>

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
        v-if="status?.error && view !== 'online'"
        type="error"
        variant="tonal"
        density="compact"
        class="mt-3"
      >
        {{ status.error }}
        <div
          v-if="status.downSince"
          class="text-caption"
        >
          {{ $t('node.downFor', { time: fmtDuration(now - status.downSince) }) }}
        </div>
      </v-alert>
      <div
        v-if="node.desc"
        class="text-medium-emphasis mt-3"
      >
        {{ node.desc }}
      </div>
    </v-card-text>
    <v-divider />
    <v-card-actions>
      <v-btn
        icon="mdi-chart-box-outline"
        @click="emit('action', 'details')"
      >
        <v-icon />
        <v-tooltip
          activator="parent"
          location="top"
          :text="$t('node.details')"
        />
      </v-btn>
      <v-btn
        icon="mdi-file-edit"
        @click="emit('action', 'edit')"
      >
        <v-icon />
        <v-tooltip
          activator="parent"
          location="top"
          :text="$t('actions.edit')"
        />
      </v-btn>
      <v-btn
        icon="mdi-sync"
        :loading="busy"
        :disabled="!live"
        @click="emit('action', 'sync')"
      >
        <v-icon />
        <v-tooltip
          activator="parent"
          location="top"
          :text="$t('node.reconcile')"
        />
      </v-btn>
      <v-spacer />
      <NodeMenu
        :node="node"
        :status="status"
        :busy="busy"
        @pick="(key: string) => emit('action', key)"
      />
    </v-card-actions>
  </v-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import NodeMenu from './NodeMenu.vue'
import { fmtBytes, fmtDate, fmtDuration, fmtPercent, fmtSpeed, fmtTime, fmtUptime, loadColor } from './format'
import { warningText } from './warnings'
import { capPercent, flagEmoji, nodeView, usage, viewColor, viewIcon, viewLabelKey, type Node, type NodeStatus } from '@/types/node'
import { i18n } from '@/locales'

const props = defineProps<{
  node: Node
  status?: NodeStatus
  selectable?: boolean
  selected?: boolean
  busy?: boolean
  // Seconds since the epoch, ticking, for the outage length.
  now: number
}>()
const emit = defineEmits<{ select: [value: boolean]; action: [key: string] }>()

const view = computed(() => nodeView(props.node, props.status))
const live = computed(() => (props.node.enable && props.status?.state === 'online' ? props.status : undefined))
const traffic = computed(() => props.status?.traffic)
const cap = computed(() => capPercent(traffic.value))
const flag = computed(() => flagEmoji(props.node.country))
const restricted = computed(() => (props.node.access?.groups?.length ?? 0) > 0 || (props.node.access?.clients?.length ?? 0) > 0)
const hasChips = computed(() => (props.node.tags?.length ?? 0) > 0 || !!props.status?.hidden || !!props.status?.filtered || !!props.node.dirty || restricted.value || !!props.node.hideDown)
const warnings = computed(() => (props.node.enable ? (props.status?.warnings ?? []).map(warningText) : []))

const bars = computed(() => {
  const s = live.value
  const size = (m?: { current: number; total: number }) => (m && m.total > 0 ? `${fmtBytes(m.current)} / ${fmtBytes(m.total)}` : '')
  const cpuHint = s?.cpuCount ? `${s.cpuCount} ${i18n.global.t('node.cores')}${s.cpuType ? ' · ' + s.cpuType : ''}` : ''
  return [
    { label: 'CPU', value: s ? Math.min(100, Math.max(0, s.cpu)) : null, hint: cpuHint },
    { label: 'RAM', value: s ? usage(s.mem) : null, hint: size(s?.mem) },
    { label: i18n.global.t('node.disk'), value: s ? usage(s.disk) : null, hint: size(s?.disk) },
  ]
})
</script>

<style scoped>
.node-selected {
  outline: 2px solid rgb(var(--v-theme-primary));
}
.node-name {
  cursor: pointer;
}
.node-bar-label {
  min-width: 2.6rem;
  font-size: .8rem;
}
.node-bar-value {
  min-width: 2.6rem;
  text-align: end;
  font-size: .8rem;
}
</style>
