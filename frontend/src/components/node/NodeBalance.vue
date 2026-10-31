<template>
  <v-expansion-panels
    v-model="open"
    class="mb-3"
  >
    <v-expansion-panel
      value="balance"
      rounded="xl"
    >
      <v-expansion-panel-title>
        <v-icon
          icon="mdi-scale-balance"
          class="me-2"
        />
        {{ $t('node.balance.title') }}
        <v-chip
          size="small"
          label
          class="ms-3"
          :color="subNodeRoute.enable ? 'success' : undefined"
          variant="tonal"
        >
          {{ modeLabel }}
        </v-chip>
        <v-chip
          v-if="dirty"
          size="small"
          label
          color="warning"
          variant="tonal"
          class="ms-2"
        >
          {{ $t('node.balance.unsaved') }}
        </v-chip>
      </v-expansion-panel-title>
      <v-expansion-panel-text>
        <v-row>
          <v-col cols="12">
            <v-switch
              :model-value="subNodeRoute.enable"
              color="primary"
              :label="$t(subNodeRoute.smart ? 'setting.balanceRouteTitle' : 'setting.subNodeRoute')"
              :hint="$t(subNodeRoute.smart ? 'setting.balanceRouteHint' : 'setting.subNodeRouteHint')"
              persistent-hint
              @update:model-value="setRouteEnabled"
            />
          </v-col>
          <v-col
            v-if="!subNodeRoute.enable"
            cols="12"
          >
            <v-switch
              v-model="subLoadOrder"
              color="primary"
              :label="$t('setting.subLoadOrder')"
              hide-details
            />
          </v-col>
          <v-col
            v-if="subNodeRoute.enable"
            cols="12"
          >
            <v-switch
              :model-value="subNodeRoute.smart"
              color="primary"
              :label="$t('setting.balanceSmart')"
              :hint="$t('setting.balanceSmartHint')"
              persistent-hint
              class="mb-4"
              @update:model-value="setSmartRoute"
            />
            <v-alert
              v-if="subNodeRoute.smart"
              type="info"
              variant="tonal"
              class="mb-4"
            >
              {{ $t('setting.balanceSingboxHint') }}
            </v-alert>
            <v-card
              v-for="(row, idx) in subNodeRoute.entries"
              :key="row.id"
              variant="outlined"
              class="pa-3 mb-3 node-balance-card"
            >
              <div class="d-flex align-center flex-wrap ga-2">
                <v-btn
                  v-if="!subNodeRoute.smart"
                  icon="mdi-arrow-up"
                  variant="text"
                  size="small"
                  :disabled="idx === 0"
                  :aria-label="$t('setting.subNodeRouteMoveUp')"
                  @click="moveRouteEntry(idx, -1)"
                />
                <v-btn
                  v-if="!subNodeRoute.smart"
                  icon="mdi-arrow-down"
                  variant="text"
                  size="small"
                  :disabled="idx === subNodeRoute.entries.length - 1"
                  :aria-label="$t('setting.subNodeRouteMoveDown')"
                  @click="moveRouteEntry(idx, 1)"
                />
                <div class="flex-grow-1">
                  {{ routeNodeName(row.id) }}
                  <span
                    v-if="!subNodeRoute.smart || !row.weight"
                    class="text-medium-emphasis ml-2"
                  >{{ routeShareLabel(row) }}</span>
                </div>
                <v-text-field
                  :model-value="row.weight"
                  type="number"
                  min="0"
                  :max="subNodeRoute.smart ? 10000 : 100"
                  :suffix="subNodeRoute.smart ? '×' : '%'"
                  hide-details
                  density="compact"
                  style="max-width: 13rem; min-width: 10rem"
                  :label="$t(subNodeRoute.smart ? 'setting.balanceWeight' : 'setting.subNodeRouteWeight')"
                  @update:model-value="(v: string) => setRouteWeight(idx, v)"
                />
                <v-btn
                  icon="mdi-close"
                  variant="text"
                  size="small"
                  :aria-label="$t('setting.subNodeRouteRemove')"
                  @click="removeRouteEntry(idx)"
                />
              </div>
              <template v-if="subNodeRoute.smart">
                <p class="text-body-2 mt-3 mb-1">
                  {{ routeHardware(row.id) }}
                </p>
                <p class="text-body-2 text-medium-emphasis mb-3">
                  {{ routeBalance(row.id) }}
                </p>
                <v-row dense>
                  <v-col
                    cols="12"
                    sm="4"
                  >
                    <v-text-field
                      :model-value="row.capacity || ''"
                      type="number"
                      min="0"
                      max="10000"
                      step="0.25"
                      :label="$t('setting.balanceCapacity')"
                      :hint="$t('setting.balanceCapacityHint')"
                      persistent-hint
                      @update:model-value="(v: string) => setRouteCapacity(idx, 'capacity', v)"
                    />
                  </v-col>
                  <v-col
                    cols="12"
                    sm="4"
                  >
                    <v-text-field
                      :model-value="row.bandwidthMbps || ''"
                      type="number"
                      min="0"
                      max="1000000"
                      suffix="Mbps"
                      :label="$t('setting.balanceBandwidth')"
                      :hint="$t('setting.balanceBandwidthHint')"
                      persistent-hint
                      @update:model-value="(v: string) => setRouteCapacity(idx, 'bandwidthMbps', v)"
                    />
                  </v-col>
                  <v-col
                    cols="12"
                    sm="4"
                  >
                    <v-text-field
                      :model-value="row.maxUsers || ''"
                      type="number"
                      min="0"
                      max="1000000"
                      :label="$t('setting.balanceMaxUsers')"
                      :hint="$t('setting.balanceMaxUsersHint')"
                      persistent-hint
                      @update:model-value="(v: string) => setRouteCapacity(idx, 'maxUsers', v)"
                    />
                  </v-col>
                </v-row>
                <v-switch
                  :model-value="!!row.drain"
                  color="warning"
                  :label="$t('setting.balanceDrain')"
                  :hint="$t('setting.balanceDrainHint')"
                  persistent-hint
                  @update:model-value="(v: boolean | null) => setRouteCapacity(idx, 'drain', !!v)"
                />
              </template>
            </v-card>
            <div
              v-if="subNodeRoute.entries.length > 0 && !subNodeRoute.smart"
              class="d-flex align-center flex-wrap ga-2 mb-3"
            >
              <span :class="routeShareSum === 100 ? 'text-success' : 'text-warning'">
                {{ $t('setting.subNodeRouteSum', { sum: routeShareSum }) }}
              </span>
              <span
                v-if="routeShareSum !== 100 && routeShareSum > 0"
                class="text-medium-emphasis text-caption"
              >{{ $t('setting.subNodeRouteSumHint') }}</span>
              <v-btn
                size="small"
                variant="tonal"
                @click="equalRouteShares"
              >
                {{ $t('setting.subNodeRouteEqual') }}
              </v-btn>
              <v-btn
                v-if="routeShareSum !== 100 && routeShareSum > 0"
                size="small"
                variant="tonal"
                @click="scaleRouteShares"
              >
                {{ $t('setting.subNodeRouteScale') }}
              </v-btn>
            </div>
            <div
              v-if="subNodeRoute.entries.length === 0"
              class="text-medium-emphasis mb-2"
            >
              {{ $t('setting.subNodeRouteNone') }}
            </div>
            <v-select
              v-if="routeAddable.length > 0"
              :items="routeAddable"
              item-title="name"
              item-value="id"
              :label="$t('setting.subNodeRouteAdd')"
              hide-details
              density="compact"
              style="max-width: 24rem"
              @update:model-value="addRouteEntry"
            />
          </v-col>
        </v-row>
        <div class="d-flex justify-end ga-2 mt-2">
          <v-btn
            variant="text"
            :disabled="!dirty || saving"
            @click="reset"
          >
            {{ $t('reset') }}
          </v-btn>
          <v-btn
            color="primary"
            variant="flat"
            :disabled="!dirty"
            :loading="saving"
            @click="save"
          >
            {{ $t('actions.save') }}
          </v-btn>
        </div>
      </v-expansion-panel-text>
    </v-expansion-panel>
  </v-expansion-panels>
</template>

<script setup lang="ts">
// How new subscriptions are spread over the nodes. This used to sit on the
// Subscription tab of the Settings page; it is the same two settings
// (subNodeRoute and subLoadOrder), now edited where the nodes are.
import { computed, onMounted, ref } from 'vue'
import { push } from 'notivue'
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import Data from '@/store/modules/data'
import { splitShares, sumShares } from '@/plugins/routeShares'
import { parseNodeRoute, serializeNodeRoute, normalizeRouteEntry, type NodeRoute, type NodeRouteEntry } from '@/plugins/nodeRoute'

type BalanceSettings = { subNodeRoute: string; subLoadOrder: string }
const keys: (keyof BalanceSettings)[] = ['subNodeRoute', 'subLoadOrder']

const open = ref<string | undefined>(undefined)
const saving = ref(false)
const current = ref<BalanceSettings>({ subNodeRoute: '{}', subLoadOrder: 'false' })
const saved = ref<BalanceSettings>({ subNodeRoute: '{}', subLoadOrder: 'false' })

const pick = (all: Record<string, string>): BalanceSettings => ({
  subNodeRoute: all.subNodeRoute || '{}',
  subLoadOrder: all.subLoadOrder || 'false',
})

const load = async () => {
  const msg = await HttpUtils.get<Record<string, string>>('api/settings')
  if (msg.success) {
    current.value = pick(msg.obj)
    saved.value = { ...current.value }
  }
}
onMounted(load)

const dirty = computed(() => keys.some(k => current.value[k] !== saved.value[k]))

const reset = () => { current.value = { ...saved.value } }

// Only these two keys are sent, so the rest of the settings -- possibly
// changed meanwhile on the Settings page or by the bot -- are left alone.
const save = async () => {
  const changed: Record<string, string> = {}
  for (const k of keys) {
    if (current.value[k] !== saved.value[k]) changed[k] = current.value[k]
  }
  if (Object.keys(changed).length === 0) return
  saving.value = true
  const msg = await HttpUtils.post<{ settings: Record<string, string> }>('api/save', {
    object: 'settings', action: 'set', data: JSON.stringify(changed),
  })
  saving.value = false
  if (msg.success) {
    current.value = pick(msg.obj.settings)
    saved.value = { ...current.value }
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t('node.balance.saved'),
    })
  }
}

const subLoadOrder = computed({
  get: () => current.value.subLoadOrder === 'true',
  set: (v: boolean | null) => { current.value.subLoadOrder = v ? 'true' : 'false' },
})

const subNodeRoute = computed({
  get: () => parseNodeRoute(current.value.subNodeRoute || '{}'),
  set: (v: NodeRoute) => { current.value.subNodeRoute = serializeNodeRoute(v) },
})

const modeLabel = computed(() => {
  if (!subNodeRoute.value.enable) {
    return i18n.global.t(subLoadOrder.value ? 'node.balance.modeLeastLoaded' : 'node.balance.modeOff')
  }
  return i18n.global.t(subNodeRoute.value.smart ? 'node.balance.modeSmart' : 'node.balance.modeWeights')
})

const setSmartRoute = (v: boolean | null) => {
  subNodeRoute.value = { ...subNodeRoute.value, smart: !!v }
}
const setRouteCapacity = (idx: number, field: 'capacity' | 'bandwidthMbps' | 'maxUsers' | 'drain', value: unknown) => {
  const cur = subNodeRoute.value
  const entries = cur.entries.map((e, i) => i === idx ? normalizeRouteEntry({ ...e, [field]: value }) : e)
  subNodeRoute.value = { ...cur, entries }
}
const routeHardware = (id: number) => {
  const st = Data().nodesStatus[id]
  if (!st) return i18n.global.t('setting.balanceUnknown')
  const cores = st.cpuCount ?? 0
  const ram = ((st.mem?.total ?? 0) / 1073741824).toFixed(1)
  return i18n.global.t('setting.balanceHardware', { cores, ram })
}
const routeBalance = (id: number) => {
  const b = Data().nodesStatus[id]?.balance
  if (!b) return i18n.global.t('setting.balancePending')
  return i18n.global.t('setting.balanceStatus', {
    state: i18n.global.t('setting.balanceStates.' + b.state), assigned: b.assigned,
    capacity: b.capacity.toFixed(1), free: b.free.toFixed(1),
  })
}

const routeNodes = computed(() => Data().nodes ?? [])

const setRouteEnabled = (v: boolean | null) => {
  const cur = subNodeRoute.value
  let entries = cur.entries
  if (v && entries.length === 0) {
    entries = [...routeNodes.value]
      .sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0) || a.name.localeCompare(b.name))
      .map(n => ({ id: n.id, weight: 0 }))
    entries = splitShares(entries.map(e => ({ ...e, weight: 1 })))
  }
  subNodeRoute.value = { ...cur, enable: !!v, entries }
}

const routeShareSum = computed(() => sumShares(subNodeRoute.value.entries))

const equalRouteShares = () => {
  const cur = subNodeRoute.value
  subNodeRoute.value = { ...cur, entries: splitShares(cur.entries.map(e => ({ ...e, weight: 1 }))) }
}

const scaleRouteShares = () => {
  const cur = subNodeRoute.value
  subNodeRoute.value = { ...cur, entries: splitShares(cur.entries) }
}

const setRouteWeight = (idx: number, v: string | number) => {
  const next = subNodeRoute.value.entries.map((e, i) => i === idx
    ? { ...e, weight: Math.max(0, Math.min(10000, Math.trunc(Number(v) || 0))) }
    : e)
  subNodeRoute.value = { ...subNodeRoute.value, entries: next }
}

const routeNodeName = (id: number) => {
  const n = routeNodes.value.find(x => x.id === id)
  return n?.name || ('#' + id)
}

const routeShareLabel = (row: NodeRouteEntry) => {
  const total = subNodeRoute.value.entries.reduce((s, e) => s + Math.max(0, Number(e.weight) || 0), 0)
  if (subNodeRoute.value.smart && row.weight > 0) return i18n.global.t('setting.balanceFactor', { factor: row.weight })
  if (!row.weight || total <= 0) return i18n.global.t('setting.subNodeRouteBackup')
  const pct = Math.round((row.weight / total) * 100)
  return i18n.global.t('setting.subNodeRouteShare', { pct })
}

const routeAddable = computed(() => {
  const used = new Set(subNodeRoute.value.entries.map(e => e.id))
  return routeNodes.value.filter(n => !used.has(n.id)).map(n => ({ id: n.id, name: n.name }))
})

const moveRouteEntry = (idx: number, dir: number) => {
  const next = [...subNodeRoute.value.entries]
  const j = idx + dir
  if (j < 0 || j >= next.length) return
  const tmp = next[idx]
  next[idx] = next[j]
  next[j] = tmp
  subNodeRoute.value = { ...subNodeRoute.value, entries: next }
}

const removeRouteEntry = (idx: number) => {
  const next = subNodeRoute.value.entries.filter((_, i) => i !== idx)
  subNodeRoute.value = { ...subNodeRoute.value, entries: next }
}

const addRouteEntry = (id: number) => {
  if (!id) return
  if (subNodeRoute.value.entries.some(e => e.id === id)) return
  subNodeRoute.value = {
    ...subNodeRoute.value,
    entries: [...subNodeRoute.value.entries, { id, weight: 100 }],
  }
}
</script>

<style scoped>
.node-balance-card :deep(.v-messages) {
  font-size: 14px;
  line-height: 1.5;
}
</style>
