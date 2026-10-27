<template>
  <v-alert
    v-for="w in warnings"
    :key="w.key"
    :type="w.type"
    variant="tonal"
    density="compact"
    class="mb-3"
  >
    {{ w.text }}
    <template #append>
      <v-btn
        size="small"
        variant="text"
        @click="emit('go', w.tab)"
      >
        {{ $t('botDash.fix') }}
      </v-btn>
    </template>
  </v-alert>

  <v-row>
    <v-col
      v-for="c in cards"
      :key="c.key"
      cols="6"
      sm="4"
      md="3"
    >
      <v-card
        rounded="xl"
        elevation="3"
        class="pa-3 h-100"
        :style="c.tab ? 'cursor: pointer' : ''"
        @click="c.tab && emit('go', c.tab)"
      >
        <div class="d-flex align-center ga-2">
          <v-icon
            :icon="c.icon"
            :color="c.color"
          />
          <span class="text-caption text-medium-emphasis">{{ c.title }}</span>
        </div>
        <div class="text-h6 mt-1">
          {{ c.value }}
        </div>
        <div
          v-if="c.sub"
          class="text-caption"
        >
          {{ c.sub }}
        </div>
      </v-card>
    </v-col>
  </v-row>

  <v-row class="mt-2">
    <v-col
      cols="12"
      md="7"
    >
      <v-card
        rounded="xl"
        elevation="3"
      >
        <v-card-title class="text-subtitle-1">
          📋 {{ $t('botDash.pendingTitle') }}
        </v-card-title>
        <v-card-subtitle>{{ $t('botDash.pendingHint') }}</v-card-subtitle>
        <v-data-table
          :items="dash.orders"
          :headers="orderHeaders"
          :loading="busy"
          density="compact"
          items-per-page="10"
          :no-data-text="$t('botDash.noPending')"
          hover
          @click:row="emit('go', 'orders')"
        >
          <template #item.customer="{ item }">
            {{ item.customer || '-' }}
            <div class="text-caption">
              <bdi>{{ item.tgId }}</bdi>
            </div>
          </template>
          <template #item.topic="{ item }">
            {{ $t('botDash.topic.' + (['buy', 'renew', 'wallet'].includes(item.topic) ? item.topic : 'other')) }}
          </template>
          <template #item.amount="{ item }">
            {{ item.amount ? fmtNum(item.amount) : '-' }}
          </template>
          <template #item.adminId="{ item }">
            {{ sellerName(item.adminId) }}
          </template>
          <template #item.createdAt="{ item }">
            {{ item.createdAt ? new Date(item.createdAt * 1000).toLocaleString() : '-' }}
          </template>
        </v-data-table>
      </v-card>
    </v-col>
    <v-col
      cols="12"
      md="5"
    >
      <v-card
        rounded="xl"
        elevation="3"
      >
        <v-card-title class="text-subtitle-1">
          👮 {{ $t('botDash.sellersTitle') }}
        </v-card-title>
        <v-table density="compact">
          <thead>
            <tr>
              <th>{{ $t('botAdmins.admin') }}</th>
              <th>{{ $t('botAdmins.customers') }}</th>
              <th>{{ $t('botAdmins.sales') }}</th>
              <th>{{ $t('botDash.owed') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="a in admins"
              :key="a.id"
              style="cursor: pointer"
              @click="emit('go', 'sellers')"
            >
              <td>
                {{ a.role == 'owner' ? '👑' : '' }} {{ a.name || a.id }}
                <div
                  v-if="a.group"
                  class="text-caption"
                >
                  🏷 {{ a.group }}
                </div>
              </td>
              <td>{{ a.customers }}</td>
              <td>
                {{ a.sales }}<span
                  v-if="a.revenue"
                  class="text-caption"
                > ({{ fmtNum(a.revenue) }})</span>
              </td>
              <td>{{ a.owed ? fmtNum(a.owed) : '-' }}</td>
            </tr>
            <tr v-if="admins.length == 0">
              <td
                colspan="4"
                class="text-medium-emphasis"
              >
                {{ $t('botAdmins.none') }}
              </td>
            </tr>
          </tbody>
        </v-table>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { computed, onMounted, ref } from 'vue'

// The bot at a glance: is it running, what is waiting, how sales go.

interface Order { id: number, tgId: number, customer: string, topic: string, detail: string, amount: number, adminId: number, createdAt: number }
interface Dash {
  enabled: boolean, hasToken: boolean, owner: number
  customers: number, blocked: number, sellers: number, pending: number
  todayOrders: number, todayRevenue: number, monthOrders: number, monthRevenue: number
  wallets: number, walletTotal: number, owed: number
  orders: Order[]
}
interface Admin { id: number, name: string, role: string, group: string, customers: number, sales: number, revenue: number, owed: number }

const emit = defineEmits<{ go: [tab: string] }>()
const t = (key: string, args: Record<string, unknown> = {}) => i18n.global.t(key, args)
const fmtNum = (n: number): string => (n ?? 0).toLocaleString('en-US')

const dash = ref<Dash>({
  enabled: true, hasToken: true, owner: 1, customers: 0, blocked: 0, sellers: 0, pending: 0,
  todayOrders: 0, todayRevenue: 0, monthOrders: 0, monthRevenue: 0, wallets: 0, walletTotal: 0, owed: 0, orders: [],
})
const admins = ref<Admin[]>([])
const busy = ref(false)

const load = async () => {
  busy.value = true
  const [d, a] = await Promise.all([
    HttpUtils.get<Dash>('api/botDashboard'),
    HttpUtils.get<{ admins: Admin[] }>('api/botAdmins'),
  ])
  busy.value = false
  if (d.success && d.obj) dash.value = { ...d.obj, orders: d.obj.orders ?? [] }
  if (a.success && a.obj) admins.value = a.obj.admins ?? []
}
onMounted(load)
defineExpose({ load })

const sellerName = (id: number): string => {
  if (!id) return t('botDash.ownerShop')
  const a = admins.value.find(x => x.id == id)
  return a ? (a.name || String(a.id)) : String(id)
}

const warnings = computed(() => {
  const out: { key: string, type: 'warning' | 'error' | 'info', text: string, tab: string }[] = []
  const d = dash.value
  if (!d.hasToken) out.push({ key: 'token', type: 'error', text: t('botDash.noToken'), tab: 'settings' })
  else if (!d.enabled) out.push({ key: 'off', type: 'warning', text: t('botDash.off'), tab: 'settings' })
  if (!d.owner) out.push({ key: 'owner', type: 'warning', text: t('botDash.noOwner'), tab: 'settings' })
  return out
})

const cards = computed(() => {
  const d = dash.value
  return [
    { key: 'status', icon: 'mdi-robot', color: d.enabled && d.hasToken ? 'success' : 'error', title: t('botDash.status'), value: d.enabled && d.hasToken ? t('botDash.on') : t('botDash.offShort'), sub: '', tab: 'settings' },
    { key: 'pending', icon: 'mdi-clipboard-clock', color: d.pending ? 'warning' : 'grey', title: t('botDash.pending'), value: fmtNum(d.pending), sub: '', tab: 'orders' },
    { key: 'today', icon: 'mdi-cash', color: 'success', title: t('botDash.today'), value: fmtNum(d.todayRevenue), sub: t('botDash.orders', { n: d.todayOrders }), tab: '' },
    { key: 'month', icon: 'mdi-calendar-month', color: 'success', title: t('botDash.month'), value: fmtNum(d.monthRevenue), sub: t('botDash.orders', { n: d.monthOrders }), tab: '' },
    { key: 'customers', icon: 'mdi-account-multiple', color: 'primary', title: t('botDash.customers'), value: fmtNum(d.customers), sub: d.blocked ? t('botDash.blocked', { n: d.blocked }) : '', tab: 'customers' },
    { key: 'sellers', icon: 'mdi-account-tie', color: 'primary', title: t('botDash.sellers'), value: fmtNum(d.sellers), sub: '', tab: 'sellers' },
    { key: 'wallets', icon: 'mdi-wallet', color: 'info', title: t('botDash.wallets'), value: fmtNum(d.walletTotal), sub: t('botDash.walletsSub', { n: d.wallets }), tab: 'customers' },
    { key: 'owed', icon: 'mdi-scale-balance', color: d.owed ? 'warning' : 'grey', title: t('botDash.owed'), value: fmtNum(d.owed), sub: '', tab: 'sellers' },
  ]
})

const orderHeaders = computed(() => [
  { title: '#', key: 'id' },
  { title: t('botDash.customer'), key: 'customer', sortable: false },
  { title: t('botDash.topicTitle'), key: 'topic' },
  { title: t('botDash.detail'), key: 'detail', sortable: false },
  { title: t('botDash.amount'), key: 'amount' },
  { title: t('botDash.seller'), key: 'adminId' },
  { title: t('botDash.date'), key: 'createdAt' },
])
</script>
