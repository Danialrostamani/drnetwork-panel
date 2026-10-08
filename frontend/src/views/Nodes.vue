<template>
  <NodeModal
    v-model="editor.visible"
    :visible="editor.visible"
    :item="editor.item"
    @close="editor.visible = false"
  />
  <NodeImport
    v-model="importer.visible"
    :visible="importer.visible"
    :node-id="importer.id"
    :node-name="importer.name"
    @close="importer.visible = false"
  />
  <NodeDetails
    v-model="details.visible"
    :visible="details.visible"
    :node-id="details.id"
    :initial-tab="details.tab"
    @close="details.visible = false"
  />
  <NodeWizard
    v-model="wizard"
    :visible="wizard"
    @close="wizard = false"
    @manual="wizard = false; edit(null)"
    @import="(id: number, name: string) => { wizard = false; openImport(id, name) }"
  />
  <NodeMultiAdd
    v-model="multi"
    :visible="multi"
    @close="multi = false"
  />
  <NodeBackup
    v-model="backup.visible"
    :visible="backup.visible"
    :node-id="backup.id"
    :node-name="backup.name"
    @close="backup.visible = false"
  />

  <v-row
    justify="center"
    align="center"
    class="mb-2"
  >
    <v-col cols="auto">
      <v-btn
        color="primary"
        prepend-icon="mdi-server-plus"
        @click="wizard = true"
      >
        {{ $t('actions.add') }} {{ $t('objects.node') }}
      </v-btn>
    </v-col>
    <v-col cols="auto">
      <v-menu location="bottom center">
        <template #activator="{ props: activator }">
          <v-btn
            v-bind="activator"
            variant="text"
            icon
          >
            <v-icon
              icon="mdi-tools"
              color="primary"
            />
          </v-btn>
        </template>
        <v-list
          density="compact"
          nav
        >
          <v-list-item
            prepend-icon="mdi-file-edit-outline"
            :title="$t('node.addManual')"
            @click="edit(null)"
          />
          <v-list-item
            prepend-icon="mdi-format-list-bulleted-square"
            :title="$t('node.multi.title')"
            @click="multi = true"
          />
          <v-list-item
            prepend-icon="mdi-database-export"
            :title="$t('node.action.backupAll')"
            :disabled="nodes.length === 0"
            @click="openBackup(0, '')"
          />
        </v-list>
      </v-menu>
    </v-col>
    <v-col
      v-if="nodes.length > 0"
      cols="auto"
    >
      <v-btn-toggle
        v-model="settings.view"
        mandatory
        density="compact"
        variant="outlined"
        divided
      >
        <v-btn
          value="cards"
          icon="mdi-view-grid-outline"
          :aria-label="$t('node.viewCards')"
        />
        <v-btn
          value="table"
          icon="mdi-table"
          :aria-label="$t('node.viewTable')"
        />
      </v-btn-toggle>
    </v-col>
    <v-col
      v-if="nodes.length > 0 && settings.view === 'cards'"
      cols="auto"
    >
      <v-btn
        :variant="selecting ? 'tonal' : 'text'"
        :color="selecting ? 'primary' : undefined"
        prepend-icon="mdi-checkbox-multiple-marked-outline"
        @click="toggleSelecting"
      >
        {{ $t('node.select') }}
      </v-btn>
    </v-col>
  </v-row>

  <v-row
    v-if="nodes.length === 0"
    justify="center"
  >
    <v-col
      cols="12"
      md="7"
    >
      <v-alert
        type="info"
        variant="tonal"
        :title="$t('node.emptyTitle')"
        :text="$t('node.emptyDesc')"
      />
    </v-col>
  </v-row>

  <template v-else>
    <v-card
      rounded="xl"
      class="mb-3 pa-2"
      variant="tonal"
    >
      <div class="d-flex flex-wrap align-center ga-2">
        <v-chip
          label
          prepend-icon="mdi-server-network"
          :variant="settings.state === 'all' ? 'flat' : 'outlined'"
          @click="setState('all')"
        >
          {{ $t('node.summary.nodes') }}: {{ summary.enabled }}/{{ summary.total }}
        </v-chip>
        <v-chip
          v-for="item in stateChips"
          :key="item.state"
          label
          :color="item.color"
          :prepend-icon="item.icon"
          :variant="settings.state === item.state ? 'flat' : 'tonal'"
          @click="setState(item.state)"
        >
          {{ $t(item.label) }}: {{ item.count }}
        </v-chip>
        <v-chip
          v-if="summary.warnings + summary.hidden > 0 || settings.state === 'problem'"
          label
          color="warning"
          prepend-icon="mdi-alert"
          :variant="settings.state === 'problem' ? 'flat' : 'tonal'"
          @click="setState('problem')"
        >
          {{ $t('node.filter.problem') }}
        </v-chip>
        <v-spacer />
        <v-chip
          label
          variant="text"
          prepend-icon="mdi-account-multiple"
        >
          {{ $t('node.summary.users') }}: {{ uniqueUsers }}<span
            v-if="summary.users != uniqueUsers"
            class="text-medium-emphasis"
          >&nbsp;({{ $t('node.summary.perNode', { n: summary.users }) }})</span>
          <v-tooltip
            activator="parent"
            location="bottom"
            :text="$t('node.summary.usersHint')"
          />
        </v-chip>
        <v-chip
          label
          variant="text"
          prepend-icon="mdi-speedometer"
        >
          <span dir="ltr">↑ {{ fmtSpeed(summary.netUp) }} · ↓ {{ fmtSpeed(summary.netDown) }}</span>
        </v-chip>
        <v-chip
          label
          variant="text"
          prepend-icon="mdi-calendar-today"
        >
          {{ $t('node.trafficToday') }}:&nbsp;<span dir="ltr">↑ {{ fmtBytes(summary.todayUp) }} · ↓ {{ fmtBytes(summary.todayDown) }}</span>
        </v-chip>
        <v-chip
          v-if="summary.warnings > 0"
          label
          variant="text"
          color="warning"
          prepend-icon="mdi-alert-outline"
        >
          {{ $t('node.summary.warnings') }}: {{ summary.warnings }}
        </v-chip>
        <v-chip
          v-if="summary.hidden > 0"
          label
          variant="text"
          color="warning"
          prepend-icon="mdi-eye-off"
        >
          {{ $t('node.summary.hidden') }}: {{ summary.hidden }}
        </v-chip>
      </div>
    </v-card>

    <v-row
      align="center"
      class="mb-1"
    >
      <v-col
        cols="12"
        sm="6"
        md="3"
      >
        <v-text-field
          v-model="search"
          density="compact"
          hide-details
          clearable
          prepend-inner-icon="mdi-magnify"
          :label="$t('node.search')"
        />
      </v-col>
      <v-col
        cols="6"
        sm="3"
        md="2"
      >
        <v-select
          v-model="settings.state"
          density="compact"
          hide-details
          :label="$t('node.filter.state')"
          :items="stateItems"
        />
      </v-col>
      <v-col
        v-if="tags.length > 0"
        cols="6"
        sm="3"
        md="2"
      >
        <v-select
          v-model="settings.tag"
          density="compact"
          hide-details
          :label="$t('node.tag')"
          :items="[{ title: $t('all'), value: '' }, ...tags.map(t => ({ title: t, value: t }))]"
        />
      </v-col>
      <v-col
        v-if="countries.length > 0"
        cols="6"
        sm="3"
        md="2"
      >
        <v-select
          v-model="settings.country"
          density="compact"
          hide-details
          :label="$t('node.country')"
          :items="[{ title: $t('all'), value: '' }, ...countries.map(c => ({ title: `${flagEmoji(c)} ${c}`, value: c }))]"
        />
      </v-col>
      <v-col
        cols="6"
        sm="3"
        md="2"
      >
        <v-select
          v-model="settings.sort"
          density="compact"
          hide-details
          :label="$t('node.sort.title')"
          :items="nodeSortKeys.map(k => ({ title: $t('node.sort.' + k), value: k }))"
        >
          <template #append>
            <v-btn
              :icon="settings.desc ? 'mdi-sort-descending' : 'mdi-sort-ascending'"
              variant="text"
              size="small"
              :aria-label="$t('node.sort.direction')"
              @click="settings.desc = !settings.desc"
            />
          </template>
        </v-select>
      </v-col>
    </v-row>

    <v-alert
      v-if="selected.length > 0"
      variant="tonal"
      color="primary"
      density="compact"
      class="mb-3"
    >
      <div class="d-flex flex-wrap align-center ga-2">
        <strong>{{ $t('node.selected', { n: selected.length }) }}</strong>
        <v-btn
          size="small"
          variant="text"
          @click="selectShown"
        >
          {{ $t('node.selectAll') }}
        </v-btn>
        <v-btn
          size="small"
          variant="text"
          @click="selected = []"
        >
          {{ $t('node.clearSelection') }}
        </v-btn>
        <v-spacer />
        <v-btn
          v-for="a in bulkQuick"
          :key="a.key"
          size="small"
          variant="tonal"
          :prepend-icon="a.icon"
          :loading="bulkBusy"
          @click="request(a.key, selected)"
        >
          {{ $t('node.action.' + a.key) }}
        </v-btn>
        <v-menu location="bottom end">
          <template #activator="{ props: activator }">
            <v-btn
              v-bind="activator"
              size="small"
              variant="tonal"
              append-icon="mdi-menu-down"
              :loading="bulkBusy"
            >
              {{ $t('node.more') }}
            </v-btn>
          </template>
          <v-list
            density="compact"
            nav
          >
            <v-list-item
              v-for="a in bulkMore"
              :key="a.key"
              :prepend-icon="a.icon"
              :title="$t('node.action.' + a.key)"
              @click="request(a.key, selected)"
            />
            <v-divider class="my-1" />
            <v-list-item
              prepend-icon="mdi-delete"
              base-color="error"
              :title="$t('actions.del')"
              @click="askDelete(selected)"
            />
          </v-list>
        </v-menu>
      </div>
    </v-alert>

    <v-row
      v-if="shown.length === 0"
      justify="center"
    >
      <v-col
        cols="12"
        md="7"
      >
        <v-alert
          type="info"
          variant="tonal"
          :text="$t('node.noMatch')"
        />
      </v-col>
    </v-row>

    <v-row v-else-if="settings.view === 'cards'">
      <v-col
        v-for="node in shown"
        :key="node.id"
        cols="12"
        md="6"
        lg="4"
      >
        <NodeCard
          :node="node"
          :status="statuses[node.id]"
          :selectable="selecting"
          :selected="selected.includes(node.id)"
          :busy="!!busy[node.id]"
          :now="now"
          @select="(v: boolean) => setSelected(node.id, v)"
          @action="(key: string) => onAction(node, key)"
        />
      </v-col>
    </v-row>

    <v-data-table
      v-else
      v-model="selected"
      :headers="headers"
      :items="shown"
      item-value="id"
      show-select
      :items-per-page="50"
      :hide-default-footer="shown.length <= 50"
      :mobile="smAndDown"
      mobile-breakpoint="sm"
      density="comfortable"
      class="elevation-3 rounded"
    >
      <template #item.name="{ item }">
        <div class="d-flex align-center ga-1">
          <span v-if="flagEmoji(item.country)">{{ flagEmoji(item.country) }}</span>
          <a
            href="#"
            class="text-decoration-none font-weight-bold"
            @click.prevent="onAction(item, 'details')"
          >{{ item.name }}</a>
        </div>
        <div
          v-if="(item.tags ?? []).length > 0"
          class="d-flex flex-wrap ga-1"
        >
          <v-chip
            v-for="t in item.tags"
            :key="t"
            size="x-small"
            variant="tonal"
            color="primary"
          >
            #{{ t }}
          </v-chip>
        </div>
      </template>
      <template #item.status="{ item }">
        <v-chip
          size="small"
          label
          :color="viewColor[nodeView(item, statuses[item.id])]"
          :prepend-icon="viewIcon[nodeView(item, statuses[item.id])]"
        >
          {{ $t(viewLabelKey(nodeView(item, statuses[item.id]))) }}
        </v-chip>
        <v-icon
          v-if="(statuses[item.id]?.warnings?.length ?? 0) > 0 && item.enable"
          icon="mdi-alert"
          color="warning"
          size="small"
          class="ms-1"
        >
          <v-tooltip
            activator="parent"
            location="top"
          >
            <div
              v-for="w in statuses[item.id]?.warnings ?? []"
              :key="w.key"
            >
              {{ warningText(w) }}
            </div>
          </v-tooltip>
        </v-icon>
        <v-icon
          v-if="statuses[item.id]?.hidden"
          icon="mdi-eye-off"
          color="warning"
          size="small"
          class="ms-1"
          :title="$t('node.hidden.' + statuses[item.id]?.hidden)"
        />
      </template>
      <template #item.latency="{ item }">
        {{ liveOf(item) ? `${liveOf(item)?.latency} ms` : '-' }}
      </template>
      <template #item.cpu="{ item }">
        {{ liveOf(item) ? fmtPercent(liveOf(item)?.cpu) : '-' }}
      </template>
      <template #item.mem="{ item }">
        {{ fmtPercent(usage(liveOf(item)?.mem)) }}
      </template>
      <template #item.disk="{ item }">
        {{ fmtPercent(usage(liveOf(item)?.disk)) }}
      </template>
      <template #item.online="{ item }">
        <a
          v-if="liveOf(item)"
          href="#"
          class="text-decoration-none"
          @click.prevent="onAction(item, 'online')"
        >{{ liveOf(item)?.online ?? 0 }}</a>
        <span v-else>-</span>
      </template>
      <template #item.speed="{ item }">
        <span
          v-if="liveOf(item)"
          dir="ltr"
          class="text-no-wrap"
        >↑ {{ fmtSpeed(liveOf(item)?.netUp) }} · ↓ {{ fmtSpeed(liveOf(item)?.netDown) }}</span>
        <span v-else>-</span>
      </template>
      <template #item.today="{ item }">
        <span
          dir="ltr"
          class="text-no-wrap"
        >{{ fmtBytes((statuses[item.id]?.traffic?.todayUp ?? 0) + (statuses[item.id]?.traffic?.todayDown ?? 0)) }}</span>
      </template>
      <template #item.cap="{ item }">
        <span v-if="capPercent(statuses[item.id]?.traffic) !== null">{{ fmtPercent(capPercent(statuses[item.id]?.traffic)) }}</span>
        <span v-else>-</span>
      </template>
      <template #item.uptime="{ item }">
        {{ fmtUptime(statuses[item.id]?.uptime24) }}
      </template>
      <template #item.version="{ item }">
        <span dir="ltr">{{ nodeVersion(statuses[item.id]) || '-' }}</span>
      </template>
      <template #item.actions="{ item }">
        <div class="d-flex align-center justify-end">
          <v-btn
            icon="mdi-file-edit"
            variant="text"
            size="small"
            @click="onAction(item, 'edit')"
          />
          <NodeMenu
            :node="item"
            :status="statuses[item.id]"
            :busy="!!busy[item.id]"
            @pick="(key: string) => onAction(item, key)"
          />
        </div>
      </template>
    </v-data-table>
  </template>

  <v-dialog
    :model-value="confirm.visible"
    max-width="480"
    @update:model-value="(v: boolean) => { if (!v) confirm.visible = false }"
  >
    <v-card
      rounded="lg"
      :title="confirm.title"
    >
      <v-card-text>
        <p>{{ confirm.text }}</p>
        <div
          class="font-weight-bold mt-2"
          dir="auto"
        >
          {{ confirm.names }}
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="outlined"
          @click="confirm.visible = false"
        >
          {{ $t('no') }}
        </v-btn>
        <v-btn
          :color="confirm.danger ? 'error' : 'primary'"
          variant="tonal"
          :loading="confirm.running"
          @click="confirmYes"
        >
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import Data from '@/store/modules/data'
import NodeModal from '@/layouts/modals/Node.vue'
import NodeImport from '@/layouts/modals/NodeImport.vue'
import NodeDetails from '@/layouts/modals/NodeDetails.vue'
import NodeWizard from '@/layouts/modals/NodeWizard.vue'
import NodeMultiAdd from '@/layouts/modals/NodeMultiAdd.vue'
import NodeBackup from '@/layouts/modals/NodeBackup.vue'
import NodeCard from '@/components/node/NodeCard.vue'
import NodeMenu from '@/components/node/NodeMenu.vue'
import { fmtBytes, fmtPercent, fmtSpeed, fmtUptime } from '@/components/node/format'
import { warningText } from '@/components/node/warnings'
import { runNodeAction } from '@/components/node/actions'
import {
  capPercent, cloneNode, confirmActions, flagEmoji, matchesFilter, nodeCountries, nodeSortKeys, nodeTags, nodeVersion,
  nodeView, nodesSummary, sortNodes, usage, viewColor, viewIcon, viewLabelKey,
  type Node, type NodeAction, type NodeSortKey, type NodeStateFilter, type NodeStatus, type NodeView,
} from '@/types/node'
import { i18n } from '@/locales'

const store = Data()
const { smAndDown } = useDisplay()
const nodes = computed(() => store.nodes)
const statuses = computed((): Record<number, NodeStatus | undefined> => store.nodesStatus)

// How the page was left: kept in the browser, the search excepted.
interface ViewSettings {
  view: 'cards' | 'table'
  sort: NodeSortKey
  desc: boolean
  state: NodeStateFilter
  tag: string
  country: string
}
const settingsKey = 'nodesView'
const stateValues: NodeStateFilter[] = ['all', 'problem', 'online', 'offline', 'core-stopped', 'maintenance', 'disabled', 'pending']
function loadSettings(): ViewSettings {
  const out: ViewSettings = { view: 'cards', sort: 'order', desc: false, state: 'all', tag: '', country: '' }
  try {
    const saved = JSON.parse(localStorage.getItem(settingsKey) ?? '{}') as Partial<ViewSettings>
    if (saved.view === 'cards' || saved.view === 'table') out.view = saved.view
    if (saved.sort && nodeSortKeys.includes(saved.sort)) out.sort = saved.sort
    if (typeof saved.desc === 'boolean') out.desc = saved.desc
    if (saved.state && stateValues.includes(saved.state)) out.state = saved.state
    if (typeof saved.tag === 'string') out.tag = saved.tag
    if (typeof saved.country === 'string') out.country = saved.country
  } catch {
    // A broken entry gives the defaults.
  }
  return out
}
const settings = reactive<ViewSettings>(loadSettings())
watch(settings, v => localStorage.setItem(settingsKey, JSON.stringify(v)), { deep: true })
const search = ref('')

const tags = computed(() => nodeTags(nodes.value))
const countries = computed(() => nodeCountries(nodes.value))
// A tag or a country no node has any more filters nothing.
watch([tags, countries], ([t, c]) => {
  if (settings.tag && !t.some(x => x.toLowerCase() === settings.tag.toLowerCase())) settings.tag = ''
  if (settings.country && !c.includes(settings.country)) settings.country = ''
})

const shown = computed(() => {
  const f = { text: search.value ?? '', state: settings.state, tag: settings.tag, country: settings.country }
  const list = nodes.value.filter(n => matchesFilter(n, statuses.value[n.id], f))
  return sortNodes(list, statuses.value, settings.sort, settings.desc)
})
const summary = computed(() => nodesSummary(nodes.value, statuses.value))
// Clients online anywhere in the cluster, each once: what the home page shows.
// The per-node figures add up a client once for every node it is on.
const uniqueUsers = computed(() => Data().onlines?.user?.length ?? 0)

const stateItems = computed(() => stateValues.map(s => ({
  title: s === 'all' ? i18n.global.t('all') : s === 'problem' ? i18n.global.t('node.filter.problem') : i18n.global.t(viewLabelKey(s as NodeView)),
  value: s,
})))
const stateChips = computed(() => {
  const s = summary.value
  const all: { state: NodeView; count: number }[] = [
    { state: 'online', count: s.online }, { state: 'offline', count: s.offline }, { state: 'core-stopped', count: s.coreStopped },
    { state: 'maintenance', count: s.maintenance }, { state: 'pending', count: s.pending }, { state: 'disabled', count: s.disabled },
  ]
  return all
    .filter(x => x.count > 0 || x.state === 'online' || settings.state === x.state)
    .map(x => ({ ...x, color: viewColor[x.state], icon: viewIcon[x.state], label: viewLabelKey(x.state) }))
})
const setState = (s: NodeStateFilter) => { settings.state = settings.state === s ? 'all' : s }

const headers = computed(() => [
  { title: i18n.global.t('node.name'), key: 'name', sortable: false },
  { title: i18n.global.t('node.filter.state'), key: 'status', sortable: false },
  { title: i18n.global.t('node.latency'), key: 'latency', sortable: false },
  { title: 'CPU', key: 'cpu', sortable: false },
  { title: 'RAM', key: 'mem', sortable: false },
  { title: i18n.global.t('node.disk'), key: 'disk', sortable: false },
  { title: i18n.global.t('node.users'), key: 'online', sortable: false },
  { title: i18n.global.t('node.speed'), key: 'speed', sortable: false },
  { title: i18n.global.t('node.trafficToday'), key: 'today', sortable: false },
  { title: i18n.global.t('node.cap.short'), key: 'cap', sortable: false },
  { title: i18n.global.t('node.uptime') + ' 24h', key: 'uptime', sortable: false },
  { title: i18n.global.t('node.panelVersion'), key: 'version', sortable: false },
  { title: '', key: 'actions', sortable: false, align: 'end' as const },
])
const liveOf = (node: Node) => {
  const s = statuses.value[node.id]
  return node.enable && s?.state === 'online' ? s : undefined
}

// A clock for how long a node has been down.
const now = ref(Math.floor(Date.now() / 1000))
const ticker = setInterval(() => { now.value = Math.floor(Date.now() / 1000) }, 5000)
onBeforeUnmount(() => clearInterval(ticker))

// ---- selection ----
const selecting = ref(false)
const selected = ref<number[]>([])
watch(nodes, list => {
  const ids = new Set(list.map(n => n.id))
  selected.value = selected.value.filter(id => ids.has(id))
})
const toggleSelecting = () => {
  selecting.value = !selecting.value
  if (!selecting.value) selected.value = []
}
const setSelected = (id: number, on: boolean) => {
  selected.value = on ? [...new Set([...selected.value, id])] : selected.value.filter(x => x !== id)
}
const selectShown = () => { selected.value = [...new Set([...selected.value, ...shown.value.map(n => n.id)])] }
const bulkQuick: { key: NodeAction; icon: string }[] = [
  { key: 'probe', icon: 'mdi-radar' },
  { key: 'sync', icon: 'mdi-sync' },
]
const bulkMore: { key: NodeAction; icon: string }[] = [
  { key: 'fullSync', icon: 'mdi-sync-alert' },
  { key: 'restartSb', icon: 'mdi-restart' },
  { key: 'restartApp', icon: 'mdi-power' },
  { key: 'maintenanceOn', icon: 'mdi-wrench-clock' },
  { key: 'maintenanceOff', icon: 'mdi-wrench-check' },
  { key: 'enable', icon: 'mdi-toggle-switch' },
  { key: 'disable', icon: 'mdi-toggle-switch-off-outline' },
]

// ---- dialogs ----
const editor = reactive<{ visible: boolean; item: Node | null }>({ visible: false, item: null })
const importer = reactive({ visible: false, id: 0, name: '' })
const details = reactive({ visible: false, id: 0, tab: 'overview' })
const backup = reactive({ visible: false, id: 0, name: '' })
const wizard = ref(false)
const multi = ref(false)
const edit = (node: Node | null) => { editor.item = node; editor.visible = true }
const openImport = (id: number, name: string) => { importer.id = id; importer.name = name; importer.visible = true }
const openDetails = (node: Node, tab: string) => { details.id = node.id; details.tab = tab; details.visible = true }
const openBackup = (id: number, name: string) => { backup.id = id; backup.name = name; backup.visible = true }

const actionKeys: NodeAction[] = ['probe', 'restartSb', 'restartApp', 'maintenanceOn', 'maintenanceOff', 'enable', 'disable', 'sync', 'fullSync']
const onAction = (node: Node, key: string) => {
  switch (key) {
    case 'details': return openDetails(node, 'overview')
    case 'online': return openDetails(node, 'online')
    case 'edit': return edit(node)
    case 'import': return openImport(node.id, node.name)
    case 'clone': return edit(cloneNode(node, nodes.value.map(n => n.name)))
    case 'backup': return openBackup(node.id, node.name)
    case 'del': return askDelete([node.id])
  }
  if ((actionKeys as string[]).includes(key)) request(key as NodeAction, [node.id])
}

// ---- actions ----
const busy = ref<Record<number, boolean>>({})
const bulkBusy = computed(() => selected.value.some(id => busy.value[id]))
const namesOf = (ids: number[]) => ids.map(id => nodes.value.find(n => n.id === id)?.name ?? `#${id}`).join(', ')

interface Confirm {
  visible: boolean
  running: boolean
  title: string
  text: string
  names: string
  danger: boolean
  run: () => Promise<void>
}
const confirm = reactive<Confirm>({ visible: false, running: false, title: '', text: '', names: '', danger: false, run: async () => {} })
const ask = (title: string, text: string, ids: number[], danger: boolean, run: () => Promise<void>) => {
  Object.assign(confirm, { visible: true, running: false, title, text, names: namesOf(ids), danger, run })
}
const confirmYes = async () => {
  confirm.running = true
  try {
    await confirm.run()
  } finally {
    confirm.running = false
    confirm.visible = false
  }
}

// The ids are copied: the selection may change while the question is open.
const request = (action: NodeAction, picked: number[]) => {
  const ids = [...picked]
  if (ids.length === 0) return
  if (confirmActions.includes(action) || ids.length > 1) {
    ask(i18n.global.t('node.action.' + action), i18n.global.t('node.action.confirm', { n: ids.length }), ids,
      confirmActions.includes(action), () => run(action, ids))
    return
  }
  run(action, ids)
}

const run = async (action: NodeAction, ids: number[]): Promise<void> => {
  busy.value = { ...busy.value, ...Object.fromEntries(ids.map(id => [id, true])) }
  try {
    await runNodeAction(action, ids)
  } finally {
    busy.value = { ...busy.value, ...Object.fromEntries(ids.map(id => [id, false])) }
  }
}

const askDelete = (picked: number[]) => {
  const ids = [...picked]
  if (ids.length === 0) return
  ask(i18n.global.t('actions.del'), i18n.global.t('confirm'), ids, true, async () => {
    for (const id of ids) {
      const ok = await store.save('nodes', 'del', id)
      if (ok) selected.value = selected.value.filter(x => x !== id)
    }
  })
}
</script>
