<template>
  <v-tabs
    v-model="tab"
    density="compact"
    color="primary"
    show-arrows
  >
    <v-tab value="overview">
      {{ $t('shop.overview') }}
    </v-tab>
    <v-tab value="orders">
      {{ $t('shop.orders') }}
    </v-tab>
    <v-tab value="plans">
      {{ $t('shop.plans') }}
    </v-tab>
    <v-tab value="codes">
      {{ $t('shop.codes') }}
    </v-tab>
    <v-tab value="sms">
      {{ $t('shop.sms') }}
    </v-tab>
    <v-tab value="wallets">
      {{ $t('shop.wallets') }}
    </v-tab>
    <v-tab value="settings">
      {{ $t('pages.settings') }}
    </v-tab>
  </v-tabs>
  <div class="d-flex align-center my-2">
    <v-chip
      :color="shopOn ? 'success' : 'grey'"
      label
      size="small"
    >
      {{ shopOn ? $t('shop.open') : $t('shop.closed') }}
    </v-chip>
    <v-spacer />
    <v-btn
      icon="mdi-refresh"
      variant="tonal"
      size="small"
      :loading="loading"
      @click="load"
    />
  </div>
  <v-window
    v-if="data"
    v-model="tab"
  >
    <v-window-item value="overview">
      <v-row dense>
        <v-col
          v-for="k in kpis"
          :key="k.label"
          cols="6"
          sm="4"
          md="3"
          lg="2"
        >
          <v-card
            variant="tonal"
            rounded="lg"
          >
            <v-card-text>
              <div class="text-caption">
                {{ k.label }}
              </div>
              <div
                class="text-h6"
                dir="ltr"
              >
                {{ k.value }}
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
      <div class="text-subtitle-2 mt-4 mb-1">
        {{ $t('shop.dailySales') }}
      </div>
      <div class="shop-chart mb-4">
        <Bar
          :key="theme.global.name.value"
          :data="chartData"
          :options="chartOptions"
        />
      </div>
      <v-row>
        <v-col
          cols="12"
          md="6"
        >
          <div class="text-subtitle-2 mb-1">
            {{ $t('shop.topPlans') }}
          </div>
          <v-table density="compact">
            <tbody>
              <tr
                v-for="p in data.stats.topPlans"
                :key="p.name"
              >
                <td>{{ p.name }}</td>
                <td>{{ p.orders }}</td>
                <td dir="ltr">
                  {{ money(p.revenue) }}
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
            {{ $t('shop.topClients') }}
          </div>
          <v-table density="compact">
            <tbody>
              <tr
                v-for="c in data.stats.topConsumers"
                :key="c.name"
              >
                <td>{{ c.name }}</td>
                <td dir="ltr">
                  {{ HumanReadable.sizeFormat(c.usage) }}
                </td>
              </tr>
            </tbody>
          </v-table>
        </v-col>
      </v-row>
    </v-window-item>

    <v-window-item value="orders">
      <v-select
        v-model="status"
        :items="statusItems"
        density="compact"
        hide-details
        style="max-width: 220px;"
        class="mb-2"
        @update:model-value="load"
      />
      <v-data-table
        :headers="orderHeaders"
        :items="data.orders"
        density="compact"
        items-per-page="25"
      >
        <template #item.kind="{ item }">
          <v-icon
            v-if="item.auto"
            icon="mdi-autorenew"
            size="small"
            :title="$t('shop.autoOrder')"
          />
          {{ orderTitle(item) }}
        </template>
        <template #item.paid="{ item }">
          <span dir="ltr">{{ money(item.paid + (item.extra || 0)) }}</span>
          <div
            v-if="item.extra > 0"
            class="text-caption text-medium-emphasis"
          >
            <span dir="ltr">+{{ money(item.extra) }}</span> {{ $t('shop.toWallet') }}
          </div>
        </template>
        <template #item.status="{ item }">
          <v-chip
            :color="statusColor(item.status)"
            size="small"
            label
          >
            {{ $t('shop.status.' + item.status) }}
          </v-chip>
        </template>
        <template #item.createdAt="{ item }">
          <span dir="ltr">{{ date(item.createdAt) }}</span>
        </template>
        <template #item.actions="{ item }">
          <template v-if="item.status === 'pending' && (item.receipt || item.method === 'card')">
            <v-btn
              size="small"
              variant="text"
              color="success"
              icon="mdi-check"
              @click="decide(item.id, true)"
            />
            <v-btn
              size="small"
              variant="text"
              color="error"
              icon="mdi-close"
              @click="decide(item.id, false)"
            />
          </template>
        </template>
      </v-data-table>
    </v-window-item>

    <v-window-item value="plans">
      <v-btn
        color="primary"
        class="mb-2"
        @click="editPlan(null)"
      >
        {{ $t('shop.newPlan') }}
      </v-btn>
      <v-data-table
        :headers="planHeaders"
        :items="data.plans"
        density="compact"
      >
        <template #item.volume="{ item }">
          <span dir="ltr">{{ item.volume > 0 ? HumanReadable.sizeFormat(item.volume) : '∞' }}</span>
        </template>
        <template #item.price="{ item }">
          <span dir="ltr">{{ money(item.price) }}</span>
        </template>
        <template #item.enable="{ item }">
          <v-icon :color="item.enable ? 'success' : 'grey'">
            {{ item.enable ? 'mdi-check-circle' : 'mdi-pause-circle' }}
          </v-icon>
        </template>
        <template #item.actions="{ item }">
          <v-btn
            size="small"
            variant="text"
            icon="mdi-pencil"
            @click="editPlan(item)"
          />
          <v-btn
            size="small"
            variant="text"
            color="error"
            icon="mdi-delete"
            @click="del('planDel', item.id)"
          />
        </template>
      </v-data-table>
    </v-window-item>

    <v-window-item value="codes">
      <v-btn
        color="primary"
        class="mb-2"
        @click="editCode(null)"
      >
        {{ $t('shop.newCode') }}
      </v-btn>
      <v-data-table
        :headers="codeHeaders"
        :items="data.discounts"
        density="compact"
      >
        <template #item.kind="{ item }">
          <v-chip
            :color="item.kind === 'gift' ? 'success' : 'primary'"
            size="small"
            label
          >
            {{ item.kind === 'gift' ? $t('shop.gift') : $t('shop.discount') }}
          </v-chip>
        </template>
        <template #item.value="{ item }">
          <span dir="ltr">{{ item.kind === 'gift' ? money(item.amount) : item.percent + '%' }}</span>
        </template>
        <template #item.limits="{ item }">
          {{ codeLimits(item) }}
        </template>
        <template #item.used="{ item }">
          {{ item.used }}{{ item.maxUses > 0 ? ' / ' + item.maxUses : '' }}
        </template>
        <template #item.expiry="{ item }">
          <span dir="ltr">{{ item.expiry > 0 ? date(item.expiry) : '∞' }}</span>
        </template>
        <template #item.actions="{ item }">
          <v-btn
            size="small"
            variant="text"
            icon="mdi-pencil"
            @click="editCode(item)"
          />
          <v-btn
            size="small"
            variant="text"
            color="error"
            icon="mdi-delete"
            @click="del('discountDel', item.id)"
          />
        </template>
      </v-data-table>
    </v-window-item>

    <v-window-item value="sms">
      <v-alert
        type="info"
        variant="tonal"
        density="compact"
        class="mb-3"
      >
        {{ $t('shop.smsHint') }}
      </v-alert>
      <v-alert
        v-if="!savedSmsKey"
        type="warning"
        variant="tonal"
        density="compact"
        class="mb-3"
      >
        {{ $t('shop.smsOff') }}
      </v-alert>
      <v-text-field
        v-else
        :model-value="hookUrl"
        :label="$t('shop.smsUrl')"
        :hint="$t('shop.smsUrlHint')"
        persistent-hint
        readonly
        dir="ltr"
        append-inner-icon="mdi-content-copy"
        class="mb-3"
        @click:append-inner="copyText(hookUrl)"
      />
      <v-card
        variant="outlined"
        class="mb-3"
      >
        <v-card-text>
          <v-textarea
            v-model="smsSample"
            :label="$t('shop.smsTest')"
            rows="3"
            auto-grow
            hide-details
          />
          <div class="d-flex align-center flex-wrap mt-2">
            <v-btn
              color="primary"
              variant="tonal"
              :disabled="!smsSample.trim()"
              @click="testSms"
            >
              {{ $t('shop.smsTestBtn') }}
            </v-btn>
            <span
              v-if="smsResult"
              class="ms-3"
              :class="smsResult.ok ? 'text-success' : 'text-error'"
            >
              <template v-if="smsResult.ok">
                {{ $t('shop.smsRead') }} <span dir="ltr">{{ money(smsResult.amount) }}</span>
              </template>
              <template v-else>
                {{ $t('shop.smsNotRead') }}<span v-if="smsResult.why"> — {{ smsResult.why }}</span>
              </template>
            </span>
          </div>
        </v-card-text>
      </v-card>
      <v-data-table
        :headers="smsHeaders"
        :items="data.sms ?? []"
        density="compact"
        items-per-page="25"
      >
        <template #item.status="{ item }">
          <v-chip
            :color="smsColor(item.status)"
            size="small"
            label
          >
            {{ $t('shop.smsStatus.' + item.status) }}
          </v-chip>
        </template>
        <template #item.amount="{ item }">
          <span dir="ltr">{{ item.amount ? money(item.amount) : '—' }}</span>
        </template>
        <template #item.orderId="{ item }">
          {{ item.orderId ? '#' + item.orderId : '—' }}
        </template>
        <template #item.text="{ item }">
          <div class="sms-text">
            {{ item.text }}
          </div>
          <div
            v-if="item.note"
            class="text-caption text-medium-emphasis"
          >
            {{ item.note }}
          </div>
        </template>
        <template #item.createdAt="{ item }">
          <span dir="ltr">{{ date(item.createdAt) }}</span>
        </template>
      </v-data-table>
    </v-window-item>

    <v-window-item value="wallets">
      <v-row>
        <v-col
          cols="12"
          md="4"
        >
          <v-card
            variant="outlined"
            :title="$t('shop.walletChange')"
          >
            <v-card-text>
              <v-text-field
                v-model="wallet.tgId"
                :label="$t('shop.tgId')"
                density="compact"
              />
              <v-text-field
                v-model="wallet.amount"
                :label="$t('shop.amountHint')"
                density="compact"
              />
              <v-text-field
                v-model="wallet.note"
                :label="$t('shop.note')"
                density="compact"
              />
              <v-btn
                color="primary"
                @click="post('wallet', { tgId: wallet.tgId, amount: wallet.amount, note: wallet.note })"
              >
                {{ $t('actions.apply') }}
              </v-btn>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col
          cols="12"
          md="4"
        >
          <v-card
            variant="outlined"
            :title="$t('shop.reseller')"
          >
            <v-card-text>
              <p class="text-caption mb-2">
                {{ $t('shop.resellerHint') }}
              </p>
              <v-text-field
                v-model="reseller.tgId"
                :label="$t('shop.tgId')"
                density="compact"
              />
              <v-text-field
                v-model="reseller.percent"
                :label="$t('shop.percent')"
                type="number"
                density="compact"
              />
              <v-btn
                color="primary"
                @click="post('reseller', { tgId: reseller.tgId, percent: reseller.percent })"
              >
                {{ $t('actions.set') }}
              </v-btn>
            </v-card-text>
          </v-card>
        </v-col>
        <v-col
          cols="12"
          md="4"
        >
          <v-card
            variant="outlined"
            :title="$t('shop.block')"
          >
            <v-card-text>
              <v-text-field
                v-model="block.tgId"
                :label="$t('shop.tgId')"
                density="compact"
              />
              <v-switch
                v-model="block.blocked"
                :label="$t('shop.blocked')"
                color="error"
                density="compact"
              />
              <v-btn
                color="primary"
                @click="post('block', { tgId: block.tgId, blocked: String(block.blocked) })"
              >
                {{ $t('actions.apply') }}
              </v-btn>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </v-window-item>

    <v-window-item value="settings">
      <v-card
        variant="outlined"
        max-width="700"
      >
        <v-card-text>
          <v-switch
            v-model="settingOn"
            :label="$t('shop.enable')"
            color="primary"
            density="compact"
            :hint="$t('shop.enableHint')"
            persistent-hint
          />
          <v-textarea
            v-model="settings.shopCard"
            :label="$t('shop.card')"
            :hint="$t('shop.cardHint')"
            persistent-hint
            rows="3"
            auto-grow
            class="mt-3 mb-2"
          />
          <v-text-field
            v-model="settings.shopCurrency"
            :label="$t('shop.currency')"
          />
          <v-text-field
            v-model="settings.shopTrial"
            :label="$t('shop.trial')"
            :hint="$t('shop.trialHint')"
            persistent-hint
          />
          <v-text-field
            v-model="settings.shopRefPercent"
            :label="$t('shop.refPercent')"
            type="number"
            class="mt-2"
          />
          <v-text-field
            v-model="settings.shopSupport"
            :label="$t('shop.support')"
          />
          <v-text-field
            v-model="settings.shopPrefix"
            :label="$t('shop.prefix')"
          />
          <v-text-field
            v-model="settings.shopUniqueAmount"
            :label="$t('shop.uniqueAmount')"
            :hint="$t('shop.uniqueAmountHint')"
            persistent-hint
            type="number"
            min="0"
            max="9999"
            class="mb-2"
          />
          <v-text-field
            v-model="settings.shopSmsSecret"
            :label="$t('shop.smsKey')"
            :hint="$t('shop.smsKeyHint')"
            persistent-hint
            dir="ltr"
            append-inner-icon="mdi-refresh"
            class="mb-2"
            @click:append-inner="newSmsKey"
          />
          <v-switch
            v-model="smsRial"
            :label="$t('shop.smsRial')"
            :hint="$t('shop.smsRialHint')"
            persistent-hint
            color="primary"
            density="compact"
            class="mb-2"
          />
          <v-switch
            v-model="autoRenew"
            :label="$t('shop.autoRenew')"
            :hint="$t('shop.autoRenewHint')"
            persistent-hint
            color="primary"
            density="compact"
            class="mb-4"
          />
          <v-btn
            color="primary"
            @click="saveSettings"
          >
            {{ $t('actions.save') }}
          </v-btn>
        </v-card-text>
      </v-card>
    </v-window-item>
  </v-window>

  <v-dialog
    v-model="planDialog"
    max-width="500"
  >
    <v-card :title="plan.id ? $t('actions.edit') : $t('shop.newPlan')">
      <v-card-text>
        <v-text-field
          v-model="plan.name"
          :label="$t('shop.planName')"
        />
        <v-row dense>
          <v-col cols="6">
            <v-text-field
              v-model.number="plan.gb"
              :label="$t('shop.gb')"
              type="number"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              v-model.number="plan.days"
              :label="$t('shop.days')"
              type="number"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              v-model.number="plan.price"
              :label="$t('shop.price')"
              type="number"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              v-model.number="plan.limitIp"
              :label="$t('shop.devices')"
              type="number"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              v-model="plan.group"
              :label="$t('shop.group')"
            />
          </v-col>
          <v-col cols="6">
            <v-text-field
              v-model.number="plan.sort"
              :label="$t('shop.sort')"
              type="number"
            />
          </v-col>
        </v-row>
        <v-switch
          v-model="plan.enable"
          :label="$t('shop.onSale')"
          color="primary"
          density="compact"
        />
        <p class="text-caption">
          {{ $t('shop.planHint') }}
        </p>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="planDialog = false">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          @click="savePlan"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog
    v-model="codeDialog"
    max-width="420"
  >
    <v-card :title="code.id ? $t('actions.edit') : $t('shop.newCode')">
      <v-card-text>
        <v-btn-toggle
          v-model="code.kind"
          mandatory
          divided
          density="compact"
          color="primary"
          variant="outlined"
          class="mb-4"
        >
          <v-btn value="discount">
            {{ $t('shop.discount') }}
          </v-btn>
          <v-btn value="gift">
            {{ $t('shop.gift') }}
          </v-btn>
        </v-btn-toggle>
        <v-text-field
          v-model="code.code"
          :label="$t('shop.code')"
          dir="ltr"
        />
        <template v-if="code.kind === 'gift'">
          <v-text-field
            v-model.number="code.amount"
            :label="$t('shop.giftAmount')"
            :hint="$t('shop.giftHint')"
            persistent-hint
            type="number"
            class="mb-2"
          />
        </template>
        <template v-else>
          <v-text-field
            v-model.number="code.percent"
            :label="$t('shop.percent')"
            type="number"
          />
          <v-select
            v-model="code.planIds"
            :items="planItems"
            :label="$t('shop.codePlans')"
            :hint="$t('shop.codePlansHint')"
            persistent-hint
            multiple
            chips
            closable-chips
            class="mb-2"
          />
          <v-select
            v-model="code.kinds"
            :items="kindItems"
            :label="$t('shop.codeKinds')"
          />
          <v-switch
            v-model="code.oncePerUser"
            :label="$t('shop.oncePerUser')"
            color="primary"
            density="compact"
            hide-details
          />
        </template>
        <v-text-field
          v-model.number="code.maxUses"
          :label="$t('shop.maxUses')"
          type="number"
        />
        <v-text-field
          v-model.number="code.days"
          :label="$t('shop.validDays')"
          type="number"
        />
        <v-switch
          v-model="code.enable"
          :label="$t('shop.active')"
          color="primary"
          density="compact"
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn @click="codeDialog = false">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          @click="saveCode"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useTheme } from 'vuetify'
import { Bar } from 'vue-chartjs'
import type { ChartData } from 'chart.js'
import HttpUtils from '@/plugins/httputil'
import { HumanReadable } from '@/plugins/utils'
import Clipboard from 'clipboard'
import { push } from 'notivue'
import { i18n, locale } from '@/locales'
import { barOptions, chartColors } from '@/components/node/charts'
import type { ShopData, ShopOrder, ShopPlan, ShopDiscount, ShopSms } from '@/types/shop'

const GIB = 1024 * 1024 * 1024
const theme = useTheme()
const tab = ref('overview')
const loading = ref(false)
const data = ref<ShopData | null>(null)
const status = ref('')
const settings = ref<Record<string, string>>({})
const settingOn = ref(false)

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<ShopData>('api/shop', { days: 30, status: status.value })
  loading.value = false
  if (msg.success && msg.obj) {
    data.value = msg.obj
    settings.value = { ...msg.obj.settings }
    settingOn.value = msg.obj.settings.shopEnable === 'true'
  }
}
onMounted(load)

const shopOn = computed(() => data.value?.settings.shopEnable === 'true')
const t = (k: string) => i18n.global.t(k)
const money = (n: number) => {
  const cur = data.value?.stats.currency || t('shop.toman')
  return (n ?? 0).toLocaleString('en-US') + ' ' + cur
}
const date = (u: number) => new Date(u * 1000).toLocaleString(locale, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

const kpis = computed(() => {
  const s = data.value?.stats
  if (!s) return []
  return [
    { label: t('shop.today'), value: money(s.revenueToday) },
    { label: t('shop.days7'), value: money(s.revenue7) },
    { label: t('shop.days30'), value: money(s.revenue30) },
    { label: t('shop.allTime'), value: money(s.revenueAll) },
    { label: t('shop.orders30'), value: String(s.orders30) },
    { label: t('shop.renewals30'), value: String(s.renewals30) },
    { label: t('shop.pending'), value: String(s.pending) },
    { label: t('shop.users'), value: s.users + ' (+' + s.newUsers30 + ')' },
    { label: t('shop.trials'), value: String(s.trials) },
    { label: t('shop.walletTotal'), value: money(s.walletTotal) },
    { label: t('shop.activeClients'), value: s.activeClients + ' / ' + s.clients },
    { label: t('shop.online24h'), value: String(s.online24h) },
    { label: t('shop.expiring3d'), value: String(s.expiring3d) },
    { label: t('shop.expired'), value: String(s.expired) },
    { label: t('shop.depleted'), value: String(s.depleted) },
  ]
})

const chartData = computed((): ChartData<'bar'> => {
  const days = data.value?.stats.days ?? []
  return {
    labels: days.map(d => d.day.slice(5)),
    datasets: [{ label: t('shop.revenue'), data: days.map(d => d.revenue), backgroundColor: chartColors.users }],
  }
})
const chartOptions = computed(() => barOptions({
  text: theme.current.value.colors['on-surface'] as string,
  grid: theme.current.value.dark ? '#333333' : '#88888850',
}, v => money(v)))

const statusItems = computed(() => [
  { title: t('shop.all'), value: '' },
  ...['pending', 'approved', 'rejected', 'canceled'].map(s => ({ title: t('shop.status.' + s), value: s })),
])
const statusColor = (s: string) => ({ pending: 'warning', approved: 'success', rejected: 'error' } as Record<string, string>)[s] ?? 'grey'
const orderTitle = (o: ShopOrder) => {
  if (o.kind === 'topup') return t('shop.topup')
  if (o.kind === 'renew') return t('shop.renew') + ' ' + o.clientName + ' · ' + o.planName
  return o.planName + (o.clientName ? ' → ' + o.clientName : '')
}
const orderHeaders = computed(() => [
  { title: '#', key: 'id' },
  { title: t('shop.customer'), key: 'tgName' },
  { title: t('shop.tgId'), key: 'tgId' },
  { title: t('shop.order'), key: 'kind' },
  { title: t('shop.paid'), key: 'paid' },
  { title: t('shop.method'), key: 'method' },
  { title: t('shop.state'), key: 'status' },
  { title: t('shop.date'), key: 'createdAt' },
  { title: '', key: 'actions', sortable: false },
])
const planHeaders = computed(() => [
  { title: t('shop.planName'), key: 'name' },
  { title: t('shop.gb'), key: 'volume' },
  { title: t('shop.days'), key: 'days' },
  { title: t('shop.devices'), key: 'limitIp' },
  { title: t('shop.price'), key: 'price' },
  { title: t('shop.group'), key: 'group' },
  { title: t('shop.onSale'), key: 'enable' },
  { title: '', key: 'actions', sortable: false },
])
const codeHeaders = computed(() => [
  { title: t('shop.code'), key: 'code' },
  { title: t('shop.codeKind'), key: 'kind' },
  { title: t('shop.value'), key: 'value', sortable: false },
  { title: t('shop.limits'), key: 'limits', sortable: false },
  { title: t('shop.used'), key: 'used' },
  { title: t('shop.expiry'), key: 'expiry' },
  { title: '', key: 'actions', sortable: false },
])

const post = async (obj: string, extra: Record<string, unknown>) => {
  const msg = await HttpUtils.post('api/shop', { obj, ...extra })
  if (msg.success) await load()
  return msg.success
}
const del = (obj: string, id: number) => post(obj, { id })
const decide = async (id: number, approve: boolean) => {
  await post('decide', { id, approve: String(approve) })
}

const planDialog = ref(false)
const plan = ref({ id: 0, name: '', gb: 0, days: 30, price: 0, limitIp: 0, group: '', sort: 0, enable: true })
const editPlan = (p: ShopPlan | null) => {
  plan.value = p
    ? { id: p.id, name: p.name, gb: Math.round(p.volume / GIB * 100) / 100, days: p.days, price: p.price, limitIp: p.limitIp, group: p.group, sort: p.sort, enable: p.enable }
    : { id: 0, name: '', gb: 0, days: 30, price: 0, limitIp: 0, group: '', sort: 0, enable: true }
  planDialog.value = true
}
const savePlan = async () => {
  const p = plan.value
  const body: ShopPlan = { id: p.id, name: p.name, volume: Math.round((Number(p.gb) || 0) * GIB), days: Number(p.days) || 0, price: Number(p.price) || 0, limitIp: Number(p.limitIp) || 0, group: p.group, sort: Number(p.sort) || 0, enable: p.enable }
  if (await post('plan', { data: JSON.stringify(body) })) planDialog.value = false
}

const parseIds = (s: string) => (s || '').split(',').map(x => Number(x.trim())).filter(n => n > 0)
const planName = (id: number) => data.value?.plans.find(p => p.id === id)?.name ?? '#' + id
const planItems = computed(() => (data.value?.plans ?? []).map(p => ({ title: p.name, value: p.id })))
const kindItems = computed(() => [
  { title: t('shop.buyAndRenew'), value: '' },
  { title: t('shop.buyOnly'), value: 'buy' },
  { title: t('shop.renewOnly'), value: 'renew' },
])
const codeLimits = (d: ShopDiscount) => {
  if (d.kind === 'gift') return t('shop.oncePerUser')
  const out: string[] = []
  if (d.planIds) out.push(parseIds(d.planIds).map(planName).join(', '))
  if (d.kinds === 'buy') out.push(t('shop.buyOnly'))
  if (d.kinds === 'renew') out.push(t('shop.renewOnly'))
  if (d.oncePerUser) out.push(t('shop.oncePerUser'))
  return out.join(' · ') || '—'
}

type CodeForm = { id: number, code: string, kind: string, percent: number, amount: number, planIds: number[], kinds: '' | 'buy' | 'renew', oncePerUser: boolean, maxUses: number, days: number, used: number, expiry: number, enable: boolean }
const emptyCode = (): CodeForm => ({ id: 0, code: '', kind: 'discount', percent: 10, amount: 0, planIds: [], kinds: '', oncePerUser: false, maxUses: 0, days: 0, used: 0, expiry: 0, enable: true })
const codeDialog = ref(false)
const code = ref<CodeForm>(emptyCode())
const editCode = (d: ShopDiscount | null) => {
  code.value = d
    ? { id: d.id, code: d.code, kind: d.kind === 'gift' ? 'gift' : 'discount', percent: d.percent || 10, amount: d.amount, planIds: parseIds(d.planIds), kinds: d.kinds || '', oncePerUser: d.oncePerUser, maxUses: d.maxUses, days: 0, used: d.used, expiry: d.expiry, enable: d.enable }
    : emptyCode()
  codeDialog.value = true
}
const saveCode = async () => {
  const c = code.value
  const gift = c.kind === 'gift'
  let expiry = c.expiry
  if (Number(c.days) > 0) expiry = Math.floor(Date.now() / 1000) + Number(c.days) * 86400
  const body: ShopDiscount = {
    id: c.id, code: c.code, kind: c.kind,
    percent: gift ? 0 : Number(c.percent) || 0,
    amount: gift ? Number(c.amount) || 0 : 0,
    planIds: gift ? '' : c.planIds.join(','),
    kinds: gift ? '' : c.kinds,
    oncePerUser: gift || c.oncePerUser,
    maxUses: Number(c.maxUses) || 0, used: c.used, expiry, enable: c.enable,
  }
  if (await post('discount', { data: JSON.stringify(body) })) codeDialog.value = false
}

// ---- bank messages ----

const smsHeaders = computed(() => [
  { title: t('shop.date'), key: 'createdAt' },
  { title: t('shop.smsSender'), key: 'sender' },
  { title: t('shop.smsText'), key: 'text', sortable: false },
  { title: t('shop.amount'), key: 'amount' },
  { title: t('shop.order'), key: 'orderId' },
  { title: t('shop.state'), key: 'status' },
])
const smsColor = (s: ShopSms['status']) => ({ approved: 'success', unmatched: 'warning', error: 'error' } as Record<string, string>)[s] ?? 'grey'
const savedSmsKey = computed(() => data.value?.settings.shopSmsSecret ?? '')
const hookUrl = computed(() => {
  const base = (window as Window & { BASE_URL?: string }).BASE_URL ?? '/'
  return location.origin + (base.endsWith('/') ? base : base + '/') + 'hook/sms/' + savedSmsKey.value
})
const copyText = (txt: string) => {
  const btn = document.createElement('button')
  btn.className = 'shop-copy-btn'
  document.body.appendChild(btn)
  const clipboard = new Clipboard('.shop-copy-btn', { text: () => txt })
  const done = (ok: boolean) => {
    clipboard.destroy()
    const message = t(ok ? 'success' : 'failed') + ': ' + t('copyToClipboard')
    if (ok) push.success({ message, duration: 3000 })
    else push.error({ message, duration: 5000 })
  }
  clipboard.on('success', () => done(true))
  clipboard.on('error', () => done(false))
  btn.click()
  document.body.removeChild(btn)
}
const smsSample = ref('')
const smsResult = ref<{ amount: number, ok: boolean, why: string } | null>(null)
const testSms = async () => {
  const msg = await HttpUtils.post<{ amount: number, ok: boolean, why: string }>('api/shop', { obj: 'smsTest', text: smsSample.value })
  smsResult.value = msg.success && msg.obj ? msg.obj : null
}
const newSmsKey = () => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  const buf = new Uint8Array(32)
  crypto.getRandomValues(buf)
  settings.value.shopSmsSecret = Array.from(buf, b => chars[b % chars.length]).join('')
}
const boolSetting = (key: string) => computed({
  get: () => settings.value[key] === 'true',
  set: (v: boolean) => { settings.value[key] = String(v) },
})
const smsRial = boolSetting('shopSmsRial')
const autoRenew = boolSetting('shopAutoRenew')

const wallet = ref({ tgId: '', amount: '', note: '' })
const reseller = ref({ tgId: '', percent: '0' })
const block = ref({ tgId: '', blocked: true })

const saveSettings = async () => {
  const s = { ...settings.value, shopEnable: String(settingOn.value) }
  await post('setting', { data: JSON.stringify(s) })
}
</script>

<style scoped>
.shop-chart {
  position: relative;
  height: 260px;
}
.sms-text {
  white-space: pre-wrap;
  max-width: 420px;
  font-size: 0.85em;
}
</style>
