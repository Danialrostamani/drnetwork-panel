<template>
  <v-card
    rounded="xl"
    elevation="3"
  >
    <v-card-title class="text-subtitle-1">
      🧾 {{ $t('botOrders.title') }}
    </v-card-title>
    <v-card-subtitle class="text-wrap">
      {{ $t('botOrders.hint') }}
    </v-card-subtitle>
    <v-card-text>
      <v-chip-group
        v-model="status"
        mandatory
        selected-class="text-primary"
        class="mb-2"
      >
        <v-chip
          v-for="s in statusChips"
          :key="s.value"
          :value="s.value"
          filter
          variant="outlined"
        >
          {{ s.title }} <span class="text-caption ms-1">({{ s.count }})</span>
        </v-chip>
      </v-chip-group>
      <v-row dense>
        <v-col
          cols="12"
          sm="6"
        >
          <v-text-field
            v-model="search"
            density="compact"
            hide-details
            clearable
            prepend-inner-icon="mdi-magnify"
            :label="$t('botOrders.search')"
          />
        </v-col>
        <v-col
          cols="12"
          sm="6"
        >
          <v-select
            v-model="seller"
            density="compact"
            hide-details
            :items="sellerFilter"
            :label="$t('botDash.seller')"
          />
        </v-col>
      </v-row>
    </v-card-text>
    <v-data-table-server
      v-model:items-per-page="perPage"
      v-model:page="page"
      :items="rows"
      :items-length="total"
      :headers="headers"
      :loading="busy"
      density="compact"
      :items-per-page-options="[25, 50, 100]"
      :no-data-text="$t('botOrders.none')"
      hover
      @click:row="(_: unknown, r: { item: Row }) => openEdit(r.item)"
    >
      <template #item.customer="{ item }">
        {{ item.customer || '-' }}
        <div class="text-caption">
          <bdi>{{ item.tgId }}</bdi>
        </div>
      </template>
      <template #item.topic="{ item }">
        {{ topicLabel(item.topic) }}
      </template>
      <template #item.detail="{ item }">
        <span class="text-caption">{{ item.detail || '-' }}</span>
        <div
          v-if="item.clientName"
          class="text-caption text-primary"
        >
          👤 {{ item.clientName }}
        </div>
      </template>
      <template #item.amount="{ item }">
        {{ item.amount ? fmtNum(item.amount) : '-' }}
        <v-icon
          v-if="item.walletPaid"
          icon="mdi-wallet"
          size="x-small"
          :title="$t('botOrders.walletPaid')"
        />
      </template>
      <template #item.adminId="{ item }">
        {{ sellerName(item.adminId) }}
      </template>
      <template #item.status="{ item }">
        <v-chip
          size="small"
          :color="statusColor(item.status)"
          variant="tonal"
        >
          {{ statusLabel(item.status) }}
        </v-chip>
        <v-icon
          v-if="item.settled"
          icon="mdi-lock-check"
          size="x-small"
          :title="$t('botOrders.settled')"
        />
      </template>
      <template #item.createdAt="{ item }">
        <span class="text-caption">{{ item.createdAt ? new Date(item.createdAt * 1000).toLocaleString() : '-' }}</span>
      </template>
    </v-data-table-server>
  </v-card>

  <v-dialog
    v-model="edit.visible"
    max-width="520"
  >
    <v-card
      v-if="edit.row"
      rounded="xl"
    >
      <v-card-title>
        {{ $t('botOrders.editTitle', { id: edit.row.id }) }}
      </v-card-title>
      <v-card-subtitle class="text-wrap">
        {{ topicLabel(edit.row.topic) }} · {{ edit.row.customer || edit.row.tgId }}
        <span v-if="edit.row.clientName"> · 👤 {{ edit.row.clientName }}</span>
      </v-card-subtitle>
      <v-card-text>
        <v-alert
          v-if="edit.row.settled"
          type="info"
          variant="tonal"
          density="compact"
          class="mb-3"
          :text="$t('botOrders.settledHint')"
        />
        <v-textarea
          v-model="edit.detail"
          :label="$t('botDash.detail')"
          rows="2"
          auto-grow
          density="compact"
        />
        <v-text-field
          v-model.number="edit.amount"
          type="number"
          min="0"
          :label="$t('botDash.amount')"
          density="compact"
          :disabled="amountLocked"
          :hint="amountLocked ? $t('botOrders.amountLocked') : ''"
          persistent-hint
        />
        <v-select
          v-model="edit.seller"
          :items="sellerItems"
          :label="$t('botDash.seller')"
          density="compact"
          :disabled="edit.row.settled"
          class="mt-2"
        />
        <v-select
          v-model="edit.status"
          :items="statusItems"
          :label="$t('botOrders.status')"
          density="compact"
          :disabled="edit.row.settled"
        />
        <v-alert
          v-if="statusNote"
          type="warning"
          variant="tonal"
          density="compact"
          class="mb-2"
          :text="statusNote"
        />
        <v-checkbox
          v-if="edit.status !== edit.row.status"
          v-model="edit.notify"
          density="compact"
          hide-details
          :label="$t('botOrders.notify')"
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="edit.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="edit.saving"
          @click="saveEdit"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { computed, onMounted, ref, watch } from 'vue'

// Every order of the bot, not only the waiting ones, each of them open to be
// corrected: amount, description, seller and status.

interface Row {
  id: number, tgId: number, customer: string, topic: string, detail: string, amount: number, adminId: number
  createdAt: number, updatedAt: number, status: string, clientId: number, clientName: string
  walletPaid: boolean, ref: string, settled: boolean, receipt: boolean
}
interface Page { rows: Row[], total: number, counts: Record<string, number> }
interface Admin { id: number, name: string, role: string }

const emit = defineEmits<{ changed: [] }>()
const t = (key: string, args: Record<string, unknown> = {}) => i18n.global.t(key, args)
const fmtNum = (n: number): string => (n ?? 0).toLocaleString('en-US')

const statuses = ['pending', 'approved', 'rejected', 'cancelled', 'reset']
const status = ref<string>('pending')
const search = ref<string>('')
const seller = ref<number>(-1)
const page = ref(1)
const perPage = ref(25)
const rows = ref<Row[]>([])
const total = ref(0)
const counts = ref<Record<string, number>>({})
const busy = ref(false)
const admins = ref<Admin[]>([])

const statusLabel = (s: string) => statuses.includes(s) ? t('botOrders.st.' + s) : s
const statusColor = (s: string) => ({ pending: 'warning', approved: 'success', rejected: 'error' } as Record<string, string>)[s] ?? 'grey'
const topicLabel = (s: string) => ['buy', 'renew', 'wallet', 'test', 'bind'].includes(s) ? t('botOrders.topic.' + s) : s

const statusChips = computed(() => {
  const all = Object.values(counts.value).reduce((a, b) => a + b, 0)
  return [{ value: 'all', title: t('botOrders.all'), count: all },
    ...statuses.filter(s => s === 'pending' || s === 'approved' || s === 'rejected' || (counts.value[s] ?? 0) > 0)
      .map(s => ({ value: s, title: statusLabel(s), count: counts.value[s] ?? 0 }))]
})

const sellerName = (id: number): string => {
  if (!id) return t('botDash.ownerShop')
  const a = admins.value.find(x => x.id == id)
  return a ? (a.name || String(a.id)) : String(id)
}
const sellerItems = computed(() => [
  { title: t('botDash.ownerShop'), value: 0 },
  ...admins.value.filter(a => a.role !== 'owner').map(a => ({ title: a.name || String(a.id), value: a.id })),
])
const sellerFilter = computed(() => [{ title: t('botOrders.all'), value: -1 }, ...sellerItems.value])

let seq = 0
const load = async () => {
  const mine = ++seq
  busy.value = true
  const q: Record<string, string | number> = { limit: perPage.value, offset: (page.value - 1) * perPage.value }
  if (status.value !== 'all') q.status = status.value
  if (seller.value >= 0) q.seller = seller.value
  if (search.value?.trim()) q.q = search.value.trim()
  const msg = await HttpUtils.get<Page>('api/botOrders', q)
  if (mine !== seq) return
  busy.value = false
  if (!msg.success || !msg.obj) return
  rows.value = msg.obj.rows ?? []
  total.value = msg.obj.total ?? 0
  counts.value = msg.obj.counts ?? {}
}

const loadAdmins = async () => {
  const a = await HttpUtils.get<{ admins: Admin[] }>('api/botAdmins')
  if (a.success && a.obj) admins.value = a.obj.admins ?? []
}

let timer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  clearTimeout(timer)
  timer = setTimeout(() => { page.value = 1; load() }, 400)
})
watch([status, seller], () => { page.value = 1; load() })
watch([page, perPage], load)
onMounted(() => { load(); loadAdmins() })
defineExpose({ load })

const headers = computed(() => [
  { title: '#', key: 'id', sortable: false },
  { title: t('botDash.customer'), key: 'customer', sortable: false },
  { title: t('botDash.topicTitle'), key: 'topic', sortable: false },
  { title: t('botDash.detail'), key: 'detail', sortable: false },
  { title: t('botDash.amount'), key: 'amount', sortable: false },
  { title: t('botDash.seller'), key: 'adminId', sortable: false },
  { title: t('botOrders.status'), key: 'status', sortable: false },
  { title: t('botDash.date'), key: 'createdAt', sortable: false },
])

const edit = ref({ visible: false, saving: false, row: null as Row | null, detail: '', amount: 0, seller: 0, status: '', notify: true })

const openEdit = (r: Row) => {
  edit.value = { visible: true, saving: false, row: r, detail: r.detail, amount: r.amount, seller: r.adminId, status: r.status, notify: true }
}

const amountLocked = computed(() => {
  const r = edit.value.row
  return !!r && (r.settled || (r.status === 'approved' && (r.walletPaid || r.topic === 'wallet')))
})

const statusItems = computed(() => {
  const r = edit.value.row
  if (!r) return []
  const out = [{ title: statusLabel(r.status), value: r.status }]
  if (r.status !== 'approved' && ['pending', 'rejected', 'cancelled'].includes(r.status)) out.push({ title: t('botOrders.approve'), value: 'approved' })
  if (r.status !== 'rejected' && ['pending', 'approved', 'cancelled'].includes(r.status)) out.push({ title: t('botOrders.reject'), value: 'rejected' })
  return out
})

const statusNote = computed(() => {
  const r = edit.value.row
  const to = edit.value.status
  if (!r || to === r.status) return ''
  if (to === 'approved') return r.walletPaid && r.status !== 'pending' ? t('botOrders.reapproveWallet') : t('botOrders.approveNote')
  if (r.status === 'approved') {
    if (r.topic === 'wallet') return t('botOrders.unapproveWallet')
    return t('botOrders.unapprove') + (r.walletPaid ? ' ' + t('botOrders.refund') : '')
  }
  return r.walletPaid ? t('botOrders.refund') : ''
})

const saveEdit = async () => {
  const e = edit.value
  const r = e.row
  if (!r) return
  const data: Record<string, string | number | boolean> = { id: r.id }
  if (e.detail !== r.detail) data.detail = e.detail ?? ''
  if (!amountLocked.value && Number(e.amount) !== r.amount) data.amount = Math.max(0, Math.round(Number(e.amount) || 0))
  if (e.seller !== r.adminId) data.seller = e.seller
  if (e.status !== r.status) {
    data.status = e.status
    data.notify = e.notify
  }
  if (Object.keys(data).length === 1) {
    e.visible = false
    return
  }
  e.saving = true
  const msg = await HttpUtils.post<Row>('api/botOrderEdit', data)
  e.saving = false
  if (!msg.success) return
  e.visible = false
  await load()
  emit('changed')
}
</script>
