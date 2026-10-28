<template>
  <!-- add a seller -->
  <v-dialog
    v-model="addDialog"
    max-width="520"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botAdmins.addTitle') }}</v-card-title>
      <v-card-text>
        <v-text-field
          v-model="addForm.id"
          :label="$t('botAdmins.tgId')"
          :hint="$t('botAdmins.tgIdHint')"
          persistent-hint
          inputmode="numeric"
          class="mb-3"
        />
        <v-radio-group
          v-model="addForm.full"
          hide-details
          class="mb-2"
        >
          <v-radio
            :value="false"
            :label="$t('botAdmins.kindSeller')"
          />
          <v-radio
            :value="true"
            :label="$t('botAdmins.kindFull')"
          />
        </v-radio-group>
        <v-combobox
          v-if="!addForm.full"
          v-model="addForm.group"
          :items="groups"
          :label="$t('botAdmins.groupOptional')"
          :hint="$t('botAdmins.groupOptionalHint')"
          persistent-hint
          clearable
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="addDialog = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          :disabled="!/^\d+$/.test(addForm.id.trim())"
          @click="addSeller"
        >
          {{ $t('botAdmins.add') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- a group for every seller -->
  <v-dialog
    v-model="autoDialog"
    max-width="520"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botAdmins.auto') }}</v-card-title>
      <v-card-text>
        <p class="mb-3">
          {{ $t('botAdmins.autoText') }}
        </p>
        <v-alert
          type="warning"
          variant="tonal"
          density="compact"
          class="mb-3"
        >
          {{ $t('botAdmins.limitWarn') }}
        </v-alert>
        <v-checkbox
          v-model="autoMove"
          :label="$t('botAdmins.moveToo')"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="autoDialog = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          @click="runAuto"
        >
          {{ $t('actions.apply') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-alert
    type="info"
    variant="tonal"
    density="compact"
    class="mb-3"
  >
    {{ $t('botAdmins.ownerNote') }}
  </v-alert>
  <div class="d-flex flex-wrap ga-2 mb-3">
    <v-btn
      color="primary"
      prepend-icon="mdi-account-plus"
      @click="openAdd"
    >
      {{ $t('botAdmins.add') }}
    </v-btn>
    <v-btn
      variant="tonal"
      prepend-icon="mdi-account-group"
      :disabled="sellers.length == 0"
      @click="autoDialog = true"
    >
      {{ $t('botAdmins.auto') }}
    </v-btn>
    <v-btn
      prepend-icon="mdi-refresh"
      variant="text"
      :loading="busy"
      @click="load"
    >
      {{ $t('botMembers.reload') }}
    </v-btn>
  </div>

  <v-alert
    v-if="!busy && admins.length == 0"
    type="warning"
    variant="tonal"
    density="compact"
  >
    {{ $t('botAdmins.none') }}
  </v-alert>

  <v-expansion-panels
    v-model="open"
    variant="accordion"
  >
    <v-expansion-panel
      v-for="a in admins"
      :key="a.id"
      :value="a.id"
      rounded="lg"
    >
      <v-expansion-panel-title>
        <div class="d-flex flex-wrap align-center ga-2 w-100">
          <v-icon :icon="a.role == 'owner' ? 'mdi-crown' : 'mdi-account-tie'" />
          <b>{{ a.name || $t('botAdmins.noName') }}</b>
          <span class="text-caption text-medium-emphasis"><bdi>{{ a.id }}</bdi></span>
          <v-chip
            size="small"
            :color="roleColor(a.role)"
          >
            {{ $t('botAdmins.role.' + a.role) }}
          </v-chip>
          <v-chip
            v-if="a.group || a.salesGroup"
            size="small"
            variant="outlined"
            prepend-icon="mdi-folder-account"
          >
            {{ a.group || a.salesGroup }}
          </v-chip>
          <v-chip
            v-if="a.suspended"
            size="small"
            color="error"
          >
            {{ $t('botAdmins.suspended') }}
          </v-chip>
          <v-spacer />
          <div class="d-flex flex-wrap ga-1">
            <v-chip
              size="x-small"
              variant="tonal"
            >
              👥 <bdi class="ms-1">{{ a.customers }}</bdi>
            </v-chip>
            <v-chip
              size="x-small"
              variant="tonal"
            >
              🛒 <bdi class="ms-1">{{ a.sales }}<template v-if="a.revenue"> · {{ fmtNum(a.revenue) }}</template></bdi>
            </v-chip>
            <v-chip
              v-if="a.quota"
              size="x-small"
              variant="tonal"
              :color="a.quota.left > 0 ? '' : 'error'"
            >
              📦 <bdi class="ms-1">{{ gbText(a.quota.left) }}</bdi>
            </v-chip>
            <v-chip
              v-if="a.owed"
              size="x-small"
              variant="tonal"
              color="warning"
            >
              🧮 <bdi class="ms-1">{{ fmtNum(a.owed) }}</bdi>
            </v-chip>
          </div>
        </div>
      </v-expansion-panel-title>
      <v-expansion-panel-text>
        <v-alert
          v-if="a.role == 'owner'"
          type="success"
          variant="tonal"
          density="compact"
        >
          {{ $t('botAdmins.ownerFull') }}
        </v-alert>
        <template v-else>
          <v-alert
            v-if="a.role == 'locked'"
            type="error"
            variant="tonal"
            density="compact"
            class="mb-3"
          >
            {{ $t('botAdmins.lockedHint') }}
          </v-alert>
          <v-row>
            <!-- role -->
            <v-col
              cols="12"
              md="6"
            >
              <v-card
                variant="outlined"
                rounded="lg"
                class="pa-3 h-100"
              >
                <div class="text-subtitle-2 mb-2">
                  🔐 {{ $t('botAdmins.access') }}
                </div>
                <v-btn-toggle
                  v-model="form(a).role"
                  mandatory
                  density="compact"
                  color="primary"
                  variant="outlined"
                  divided
                  class="mb-3 flex-wrap"
                >
                  <v-btn value="group">
                    {{ $t('botAdmins.kindGroup') }}
                  </v-btn>
                  <v-btn value="sections">
                    {{ $t('botAdmins.kindSections') }}
                  </v-btn>
                  <v-btn value="full">
                    {{ $t('botAdmins.kindFullShort') }}
                  </v-btn>
                </v-btn-toggle>
                <div class="text-caption text-medium-emphasis mb-2">
                  {{ $t('botAdmins.kindHint.' + form(a).role) }}
                </div>
                <template v-if="form(a).role == 'group'">
                  <v-combobox
                    v-model="form(a).group"
                    :items="groups"
                    :label="$t('botAdmins.group')"
                    density="compact"
                    hide-details
                    class="mb-2"
                  />
                  <v-checkbox
                    v-model="form(a).move"
                    :label="$t('botAdmins.moveOwn')"
                    density="compact"
                    hide-details
                  />
                </template>
                <v-chip-group
                  v-if="form(a).role == 'sections'"
                  v-model="form(a).sections"
                  multiple
                  column
                  selected-class="text-primary"
                >
                  <v-chip
                    v-for="s in sections"
                    :key="s.key"
                    :value="s.key"
                    filter
                    size="small"
                    variant="outlined"
                  >
                    {{ s.icon }} {{ locale == 'fa' ? s.fa : s.en }}
                  </v-chip>
                </v-chip-group>
                <div class="d-flex flex-wrap ga-2 mt-2">
                  <v-btn
                    color="primary"
                    size="small"
                    :loading="saving"
                    :disabled="!roleChanged(a)"
                    @click="saveRole(a)"
                  >
                    {{ $t('actions.save') }}
                  </v-btn>
                  <v-btn
                    v-if="a.role == 'group' && a.outside > 0"
                    size="small"
                    variant="tonal"
                    color="warning"
                    prepend-icon="mdi-folder-move"
                    :loading="saving"
                    @click="moveClients(a)"
                  >
                    {{ $t('botAdmins.move', { n: a.outside }) }}
                  </v-btn>
                </div>
                <div
                  v-if="a.group || a.salesGroup"
                  class="text-caption mt-2"
                >
                  {{ $t('botAdmins.groupClients', { n: a.clients }) }}
                </div>
              </v-card>
            </v-col>

            <!-- volume limit -->
            <v-col
              cols="12"
              md="6"
            >
              <v-card
                variant="outlined"
                rounded="lg"
                class="pa-3 h-100"
              >
                <div class="text-subtitle-2 mb-2">
                  📦 {{ $t('botAdmins.quota') }}
                </div>
                <template v-if="a.quota">
                  <v-progress-linear
                    :model-value="a.quota.total > 0 ? Math.min(100, a.quota.used * 100 / a.quota.total) : 100"
                    :color="a.quota.left > 0 ? 'primary' : 'error'"
                    height="8"
                    rounded
                    class="mb-1"
                  />
                  <div class="text-caption mb-2">
                    {{ $t('botAdmins.quotaState', { used: gbText(a.quota.used), total: gbText(a.quota.total), left: gbText(a.quota.left), n: a.quota.clients }) }}
                  </div>
                </template>
                <div
                  v-else
                  class="text-caption text-medium-emphasis mb-2"
                >
                  {{ $t('botAdmins.quotaNone') }}
                </div>
                <div class="d-flex flex-wrap ga-2 align-center">
                  <v-text-field
                    v-model="form(a).gb"
                    type="number"
                    min="0"
                    :label="$t('botAdmins.gb')"
                    density="compact"
                    hide-details
                    style="max-width: 9rem"
                  />
                  <v-btn
                    size="small"
                    color="primary"
                    :disabled="!(Number(form(a).gb) > 0)"
                    :loading="saving"
                    @click="quota(a, a.quota ? 'add' : 'set')"
                  >
                    {{ a.quota ? $t('botAdmins.quotaAdd') : $t('botAdmins.quotaSet') }}
                  </v-btn>
                  <v-btn
                    v-if="a.quota"
                    size="small"
                    variant="tonal"
                    :disabled="!(Number(form(a).gb) >= 0) || form(a).gb === ''"
                    :loading="saving"
                    @click="quota(a, 'set')"
                  >
                    {{ $t('botAdmins.quotaSetTo') }}
                  </v-btn>
                </div>
                <div
                  v-if="a.quota"
                  class="d-flex flex-wrap ga-2 mt-2"
                >
                  <v-btn
                    size="small"
                    variant="text"
                    prepend-icon="mdi-restore"
                    :loading="saving"
                    @click="quota(a, 'reset')"
                  >
                    {{ $t('botAdmins.quotaReset') }}
                  </v-btn>
                  <v-btn
                    size="small"
                    variant="text"
                    color="error"
                    prepend-icon="mdi-close"
                    :loading="saving"
                    @click="quota(a, 'remove')"
                  >
                    {{ $t('botAdmins.quotaRemove') }}
                  </v-btn>
                </div>
              </v-card>
            </v-col>

            <!-- terms and settlement -->
            <v-col
              cols="12"
              md="6"
            >
              <v-card
                variant="outlined"
                rounded="lg"
                class="pa-3 h-100"
              >
                <div class="text-subtitle-2 mb-2">
                  🤝 {{ $t('botAdmins.terms') }}
                </div>
                <v-switch
                  v-model="form(a).suspended"
                  color="error"
                  :label="$t('botAdmins.suspend')"
                  density="compact"
                  hide-details
                />
                <v-select
                  v-model="form(a).route"
                  :items="routeItems(a)"
                  :label="$t('botAdmins.receiptTo')"
                  :hint="$t('botAdmins.receiptToHint')"
                  persistent-hint
                  density="compact"
                  class="mt-2 mb-3"
                />
                <div class="d-flex flex-wrap ga-2">
                  <v-text-field
                    v-model="form(a).minPerGB"
                    type="number"
                    min="0"
                    :label="$t('botAdmins.minPerGB')"
                    density="compact"
                    hide-details
                    style="max-width: 11rem"
                  />
                  <v-text-field
                    v-model="form(a).ratePerGB"
                    type="number"
                    min="0"
                    :label="$t('botAdmins.ratePerGB')"
                    density="compact"
                    hide-details
                    style="max-width: 11rem"
                  />
                </div>
                <v-btn
                  size="small"
                  color="primary"
                  class="mt-2"
                  :loading="saving"
                  :disabled="!termsChanged(a)"
                  @click="saveTerms(a)"
                >
                  {{ $t('actions.save') }}
                </v-btn>
                <v-divider class="my-3" />
                <div class="text-caption mb-2">
                  {{ $t('botAdmins.unsettled', { n: a.unsettledOrders, gb: a.unsettledGB, owed: fmtNum(a.owed) }) }}
                </div>
                <v-btn
                  size="small"
                  variant="tonal"
                  prepend-icon="mdi-cash-check"
                  :disabled="a.unsettledOrders == 0"
                  :loading="saving"
                  @click="settle(a)"
                >
                  {{ $t('botAdmins.settle') }}
                </v-btn>
              </v-card>
            </v-col>

            <!-- their own shop -->
            <v-col
              cols="12"
              md="6"
            >
              <v-card
                variant="outlined"
                rounded="lg"
                class="pa-3 h-100"
              >
                <div class="text-subtitle-2 mb-1">
                  🛍 {{ $t('botAdmins.desk') }}
                </div>
                <div class="text-caption text-medium-emphasis mb-2">
                  {{ $t('botAdmins.deskHint') }}
                </div>
                <template v-if="hasDesk(a.id)">
                  <v-combobox
                    :model-value="a.role == 'group' ? a.group : deskOf(a.id).salesGroup"
                    :items="groups"
                    :label="$t('botAdmins.salesGroup')"
                    :hint="$t('botAdmins.salesGroupHint')"
                    persistent-hint
                    clearable
                    density="compact"
                    class="mb-3"
                    :disabled="a.role == 'group'"
                    @update:model-value="(v: string | null) => patchShopDesk(a.id, { salesGroup: v ?? '' })"
                  />
                  <div class="d-flex flex-wrap ga-2 mb-2">
                    <v-text-field
                      :model-value="deskOf(a.id).payCard"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPayCard')"
                      style="min-width: 10rem"
                      @update:model-value="(v: string) => patchShopDesk(a.id, { payCard: v })"
                    />
                    <v-text-field
                      :model-value="deskOf(a.id).payWallet"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPayWallet')"
                      style="min-width: 10rem"
                      @update:model-value="(v: string) => patchShopDesk(a.id, { payWallet: v })"
                    />
                    <v-text-field
                      :model-value="deskOf(a.id).payHint"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPayHint')"
                      style="min-width: 10rem"
                      @update:model-value="(v: string) => patchShopDesk(a.id, { payHint: v })"
                    />
                    <v-text-field
                      :model-value="deskOf(a.id).support"
                      density="compact"
                      hide-details
                      dir="ltr"
                      :label="$t('botAdmins.deskSupport')"
                      placeholder="https://t.me/..."
                      style="min-width: 10rem"
                      @update:model-value="(v: string) => patchShopDesk(a.id, { support: v })"
                    />
                    <v-text-field
                      :model-value="deskOf(a.id).channel"
                      density="compact"
                      hide-details
                      dir="ltr"
                      :label="$t('botAdmins.deskChannel')"
                      placeholder="https://t.me/..."
                      style="min-width: 10rem"
                      @update:model-value="(v: string) => patchShopDesk(a.id, { channel: v })"
                    />
                  </div>
                  <div
                    v-for="(p, i) in deskOf(a.id).plans"
                    :key="p.id"
                    class="d-flex flex-wrap ga-2 mb-2"
                  >
                    <v-text-field
                      :model-value="p.name"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanName')"
                      style="min-width: 7rem"
                      @update:model-value="(v: string) => patchShopDeskPlan(a.id, i, { name: v })"
                    />
                    <v-text-field
                      :model-value="String(p.days)"
                      type="number"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanDays')"
                      style="max-width: 5.5rem"
                      @update:model-value="(v: string) => patchShopDeskPlan(a.id, i, { days: Number(v) || 0 })"
                    />
                    <v-text-field
                      :model-value="String(p.gb)"
                      type="number"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanGB')"
                      style="max-width: 6.5rem"
                      @update:model-value="(v: string) => patchShopDeskPlan(a.id, i, { gb: Number(v) || 0 })"
                    />
                    <v-text-field
                      :model-value="p.price"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanPrice')"
                      style="max-width: 8rem"
                      @update:model-value="(v: string) => patchShopDeskPlan(a.id, i, { price: v })"
                    />
                    <v-btn
                      icon="mdi-close"
                      variant="text"
                      size="small"
                      @click="removeShopDeskPlan(a.id, i)"
                    />
                  </div>
                  <div class="d-flex flex-wrap ga-2">
                    <v-btn
                      variant="tonal"
                      size="small"
                      @click="addShopDeskPlan(a.id)"
                    >
                      {{ $t('setting.tgBotShopAddPlan') }}
                    </v-btn>
                    <v-btn
                      variant="text"
                      size="small"
                      color="error"
                      @click="removeShopDesk(a.id)"
                    >
                      {{ $t('botAdmins.deskRemove') }}
                    </v-btn>
                  </div>
                </template>
                <v-btn
                  v-else
                  size="small"
                  variant="tonal"
                  prepend-icon="mdi-store-plus"
                  @click="patchShopDesk(a.id, {})"
                >
                  {{ $t('botAdmins.deskCreate') }}
                </v-btn>
              </v-card>
            </v-col>
          </v-row>
          <div class="d-flex mt-3">
            <v-spacer />
            <v-btn
              color="error"
              variant="outlined"
              size="small"
              prepend-icon="mdi-account-remove"
              :loading="saving"
              @click="removeSeller(a)"
            >
              {{ $t('botAdmins.remove') }}
            </v-btn>
          </div>
        </template>
      </v-expansion-panel-text>
    </v-expansion-panel>
  </v-expansion-panels>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { computed, onMounted, ref } from 'vue'
import { useBotSettings } from '@/plugins/botSettings'

// Everything about the bot's administrators in one place: who they are, what
// they may do, their volume limit, the owner's terms with them and their own
// shop. Role, limit and terms are saved at once; the shop with the page.

type Role = 'owner' | 'full' | 'sections' | 'group' | 'locked'
interface BotAdmin {
  id: number
  name: string
  role: Role
  group: string
  salesGroup?: string
  sections: string[]
  clients: number
  customers: number
  sales: number
  revenue: number
  outside: number
  suspended: boolean
  requireOwner: boolean
  receiptTo: number
  minPerGB: number
  ratePerGB: number
  quota: { total: number, used: number, left: number, clients: number } | null
  unsettledOrders: number
  unsettledGB: number
  owed: number
}
interface Section { key: string, icon: string, fa: string, en: string }
interface Form {
  role: 'full' | 'sections' | 'group'
  group: string
  move: boolean
  sections: string[]
  gb: string
  suspended: boolean
  // route: 0 the seller and the owner, -1 the seller alone, -2 the owner
  // alone (requireOwner), else the administrator who approves.
  route: number
  minPerGB: string
  ratePerGB: string
}

const { deskOf, hasDesk, patchShopDesk, patchShopDeskPlan, addShopDeskPlan, removeShopDeskPlan, removeShopDesk, refreshAccess } = useBotSettings()

const t = (key: string, args: Record<string, unknown> = {}) => i18n.global.t(key, args)
const locale = computed(() => String(i18n.global.locale.value))
const fmtNum = (n: number): string => (n ?? 0).toLocaleString('en-US')
const gbText = (bytes: number): string => {
  const gb = (bytes ?? 0) / 1024 ** 3
  return (gb >= 100 ? Math.round(gb) : Math.round(gb * 10) / 10) + (locale.value == 'fa' ? ' گیگ' : ' GB')
}

const admins = ref<BotAdmin[]>([])
const groups = ref<string[]>([])
const sections = ref<Section[]>([])
const forms = ref<Record<number, Form>>({})
const open = ref<number | undefined>(undefined)
const busy = ref(false)
const saving = ref(false)
const autoDialog = ref(false)
const autoMove = ref(true)
const addDialog = ref(false)
const addForm = ref({ id: '', full: false, group: '' })

const sellers = computed(() => admins.value.filter(a => a.role != 'owner'))

const formFrom = (a: BotAdmin): Form => ({
  role: a.role == 'group' || a.role == 'sections' ? a.role : (a.role == 'locked' ? 'sections' : 'full'),
  group: a.group,
  move: true,
  sections: [...(a.sections ?? [])],
  gb: '',
  suspended: a.suspended,
  route: a.requireOwner ? -2 : (a.receiptTo ?? 0),
  minPerGB: a.minPerGB ? String(a.minPerGB) : '',
  ratePerGB: a.ratePerGB ? String(a.ratePerGB) : '',
})
const form = (a: BotAdmin): Form => {
  if (!forms.value[a.id]) forms.value[a.id] = formFrom(a)
  return forms.value[a.id]
}

const currentRoute = (a: BotAdmin): number => a.requireOwner ? -2 : (a.receiptTo ?? 0)
const routeItems = (a: BotAdmin) => [
  { title: t('botAdmins.routeBoth'), value: 0 },
  { title: t('botAdmins.routeSelf'), value: -1 },
  { title: t('botAdmins.routeOwner'), value: -2 },
  ...admins.value
    .filter(x => x.id != a.id && x.role != 'owner')
    .map(x => ({ title: t('botAdmins.routeAdmin', { name: x.name || String(x.id) }), value: x.id })),
]

const roleColor = (r: string) => ({ owner: 'primary', full: 'success', sections: 'info', group: 'secondary', locked: 'error' } as Record<string, string>)[r] ?? ''

const roleChanged = (a: BotAdmin): boolean => {
  const f = form(a)
  if (a.role == 'locked') return true
  if (f.role != a.role) return f.role != 'group' || f.group.trim() != ''
  if (f.role == 'group') return f.group.trim() != '' && f.group.trim() != a.group
  if (f.role == 'sections') return [...f.sections].sort().join(',') != [...a.sections].sort().join(',')
  return false
}
const termsChanged = (a: BotAdmin): boolean => {
  const f = form(a)
  return f.suspended != a.suspended || f.route != currentRoute(a) ||
    (Number(f.minPerGB) || 0) != a.minPerGB || (Number(f.ratePerGB) || 0) != a.ratePerGB
}

const load = async () => {
  busy.value = true
  const msg = await HttpUtils.get<{ admins: BotAdmin[], groups: string[], sections: Section[] }>('api/botAdmins')
  busy.value = false
  if (msg.success && msg.obj) {
    admins.value = msg.obj.admins ?? []
    groups.value = msg.obj.groups ?? []
    sections.value = msg.obj.sections ?? []
    forms.value = Object.fromEntries(admins.value.map(a => [a.id, formFrom(a)]))
    if (open.value === undefined && sellers.value.length > 0) open.value = sellers.value[0].id
  }
}

onMounted(load)

// after reloads the list, and the access settings the page holds.
const after = async (ok: boolean, text: string) => {
  if (!ok) return
  push.success({ title: text })
  await Promise.all([load(), refreshAccess()])
}

const openAdd = () => {
  addForm.value = { id: '', full: false, group: '' }
  addDialog.value = true
}

const addSeller = async () => {
  saving.value = true
  const id = addForm.value.id.trim()
  const msg = await HttpUtils.post<{ group: string }>('api/botAdminAdd', { id, full: addForm.value.full, group: addForm.value.full ? '' : (addForm.value.group ?? '').trim() })
  saving.value = false
  if (msg.success) {
    addDialog.value = false
    await after(true, msg.obj?.group ? t('botAdmins.addedGroup', { group: msg.obj.group }) : t('botAdmins.added'))
    open.value = Number(id)
  }
}

const removeSeller = async (a: BotAdmin) => {
  if (!confirm(t('botAdmins.removeConfirm', { name: a.name || a.id }))) return
  saving.value = true
  const msg = await HttpUtils.post('api/botAdminRemove', { id: a.id })
  saving.value = false
  if (msg.success) {
    if (hasDesk(a.id) && confirm(t('botAdmins.deskRemoveToo'))) removeShopDesk(a.id)
  }
  await after(msg.success, t('botAdmins.removed'))
}

const saveRole = async (a: BotAdmin) => {
  const f = form(a)
  if (f.role == 'group') {
    const group = f.group.trim()
    if (!confirm(t('botAdmins.limitConfirm', { group }))) return
    saving.value = true
    const msg = await HttpUtils.post<{ moved: number }>('api/botAdminGroup', { id: a.id, group, move: f.move })
    saving.value = false
    await after(msg.success, t('botAdmins.saved', { n: msg.obj?.moved ?? 0 }))
    return
  }
  if (f.role == 'full' && !confirm(t('botAdmins.freeConfirm'))) return
  saving.value = true
  const msg = await HttpUtils.post('api/botAdminRole', { id: a.id, role: f.role, sections: f.sections.join(',') })
  saving.value = false
  await after(msg.success, t('botAdmins.done'))
}

const moveClients = async (a: BotAdmin) => {
  saving.value = true
  const msg = await HttpUtils.post<{ moved: number }>('api/botAdminMove', { id: a.id })
  saving.value = false
  await after(msg.success, t('botAdmins.saved', { n: msg.obj?.moved ?? 0 }))
}

const quota = async (a: BotAdmin, op: 'set' | 'add' | 'reset' | 'remove') => {
  if (op == 'remove' && !confirm(t('botAdmins.quotaRemoveConfirm'))) return
  if (op == 'reset' && !confirm(t('botAdmins.quotaResetConfirm'))) return
  saving.value = true
  const msg = await HttpUtils.post('api/botAdminQuota', { id: a.id, op, gb: Math.trunc(Number(form(a).gb) || 0) })
  saving.value = false
  await after(msg.success, t('botAdmins.done'))
}

const saveTerms = async (a: BotAdmin) => {
  const f = form(a)
  saving.value = true
  const msg = await HttpUtils.post('api/botAdminTerms', {
    id: a.id, suspended: f.suspended, requireOwner: f.route == -2,
    receiptTo: f.route == -1 || f.route > 0 ? f.route : 0,
    minPerGB: Math.trunc(Number(f.minPerGB) || 0), ratePerGB: Math.trunc(Number(f.ratePerGB) || 0),
  })
  saving.value = false
  await after(msg.success, t('botAdmins.done'))
}

const runAuto = async () => {
  saving.value = true
  const msg = await HttpUtils.post<{ id: number, group: string, new: boolean, moved: number }[]>('api/botAdminsAuto', { move: autoMove.value })
  saving.value = false
  if (msg.success) {
    const list = msg.obj ?? []
    autoDialog.value = false
    await after(true, t('botAdmins.autoDone', { n: list.filter(x => x.new).length, m: list.reduce((n, x) => n + x.moved, 0) }))
  }
}

const settle = async (a: BotAdmin) => {
  if (!confirm(t('botAdmins.settleConfirm', { owed: fmtNum(a.owed) }))) return
  saving.value = true
  const msg = await HttpUtils.post('api/botAdminSettle', { id: a.id })
  saving.value = false
  await after(msg.success, t('botAdmins.done'))
}
</script>
