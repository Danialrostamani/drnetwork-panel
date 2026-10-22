<template>
  <!-- introducer -->
  <v-dialog
    v-model="refDialog.visible"
    max-width="420"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.editRef') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }}</v-card-subtitle>
      <v-card-text>
        <v-alert
          v-if="refs.length == 0"
          type="info"
          variant="tonal"
          density="compact"
        >
          {{ $t('botMembers.noRefs') }}
        </v-alert>
        <v-select
          v-else
          v-model="refDialog.word"
          :items="refItems"
          :label="$t('botMembers.ref')"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-btn
          color="error"
          variant="outlined"
          prepend-icon="mdi-delete"
          :disabled="refDialog.current == '' || saving"
          @click="act('botMemberRef', { word: '' })"
        >
          {{ $t('botMembers.removeRef') }}
        </v-btn>
        <v-spacer />
        <v-btn
          variant="text"
          @click="refDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          :disabled="refDialog.word == '' || refDialog.word == refDialog.current"
          @click="act('botMemberRef', { word: refDialog.word })"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- seller -->
  <v-dialog
    v-model="sellerDialog.visible"
    max-width="420"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.editSeller') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }}</v-card-subtitle>
      <v-card-text>
        <v-select
          v-model="sellerDialog.seller"
          :items="sellerItems"
          :label="$t('botMembers.seller')"
          :hint="$t('botMembers.sellerHint')"
          persistent-hint
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="sellerDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          :disabled="sellerDialog.seller == sellerDialog.current"
          @click="act('botMemberSeller', { seller: sellerDialog.seller })"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- wallet -->
  <v-dialog
    v-model="walletDialog.visible"
    max-width="420"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.wallet') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }} · {{ $t('botMembers.balance') }}: {{ fmtNum(walletDialog.balance) }}</v-card-subtitle>
      <v-card-text>
        <v-btn-toggle
          v-model="walletDialog.sign"
          mandatory
          density="compact"
          color="primary"
          class="mb-3"
        >
          <v-btn value="+">
            {{ $t('botMembers.walletAdd') }}
          </v-btn>
          <v-btn value="-">
            {{ $t('botMembers.walletTake') }}
          </v-btn>
        </v-btn-toggle>
        <v-text-field
          v-model.number="walletDialog.amount"
          type="number"
          min="1"
          :label="$t('botMembers.amount')"
        />
        <v-text-field
          v-model="walletDialog.note"
          :label="$t('botMembers.walletNote')"
          maxlength="80"
        />
        <v-checkbox
          v-model="walletDialog.notify"
          :label="$t('botMembers.walletNotify')"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="walletDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          :disabled="!(walletDialog.amount > 0)"
          @click="saveWallet"
        >
          {{ $t('actions.apply') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- message -->
  <v-dialog
    v-model="msgDialog.visible"
    max-width="520"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.message') }}</v-card-title>
      <v-card-subtitle>{{ $t('botMembers.recipients', { n: msgTargets.length }) }}</v-card-subtitle>
      <v-card-text>
        <v-textarea
          v-model="msgDialog.text"
          :label="$t('botMembers.messageText')"
          :hint="$t('botMembers.messageHint')"
          persistent-hint
          rows="5"
          counter="3800"
          auto-grow
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="msgDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          prepend-icon="mdi-send"
          :loading="saving"
          :disabled="msgDialog.text.trim() == '' || msgDialog.text.length > 3800 || msgTargets.length == 0"
          @click="sendMessage"
        >
          {{ $t('botMembers.send') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- edit -->
  <v-dialog
    v-model="editDialog.visible"
    max-width="560"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.editCustomer') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }}</v-card-subtitle>
      <v-card-text>
        <v-textarea
          v-model="editDialog.note"
          :label="$t('botMembers.note')"
          :hint="$t('botMembers.noteHint')"
          persistent-hint
          rows="2"
          auto-grow
          counter="500"
          class="mb-3"
        />
        <v-autocomplete
          v-model="editDialog.clients"
          :items="clientItems"
          :label="$t('botMembers.boundClients')"
          :hint="$t('botMembers.boundHint')"
          persistent-hint
          multiple
          chips
          closable-chips
          class="mb-3"
        />
        <v-select
          v-if="refs.length"
          v-model="editDialog.ref"
          :items="[{ title: $t('botMembers.noRef'), value: '' }, ...refItems]"
          :label="$t('botMembers.ref')"
          class="mb-1"
        />
        <v-select
          v-model="editDialog.seller"
          :items="sellerItems"
          :label="$t('botMembers.seller')"
          :hint="$t('botMembers.sellerHint')"
          persistent-hint
          class="mb-3"
        />
        <v-switch
          v-model="editDialog.blocked"
          color="error"
          :label="$t('botMembers.blocked')"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="editDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          :disabled="editDialog.note.length > 500"
          @click="saveEdit"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- trial reset -->
  <v-dialog
    v-model="trialDialog.visible"
    max-width="460"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.trialReset') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }}</v-card-subtitle>
      <v-card-text>
        <p class="mb-3">
          {{ $t('botMembers.trialResetText') }}
        </p>
        <v-checkbox
          v-if="trialDialog.names.length"
          v-model="trialDialog.deleteClient"
          :label="$t('botMembers.trialDeleteClient') + ': ' + trialDialog.names.join(', ')"
          color="error"
          hide-details
        />
        <p
          v-else
          class="text-caption mb-2"
        >
          {{ $t('botMembers.trialNoClient') }}
        </p>
        <v-checkbox
          v-model="trialDialog.notify"
          :label="$t('botMembers.trialNotify')"
          hide-details
        />
        <v-alert
          v-if="trialStillBound > 0"
          type="warning"
          variant="tonal"
          density="compact"
          class="mt-2"
        >
          {{ $t('botMembers.trialStillBound', { n: trialStillBound }) }}
        </v-alert>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="trialDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="warning"
          variant="flat"
          prepend-icon="mdi-restore"
          :loading="saving"
          @click="resetTrial"
        >
          {{ $t('botMembers.trialReset') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- delete -->
  <v-dialog
    v-model="deleteDialog.visible"
    max-width="460"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botMembers.deleteCustomer') }}</v-card-title>
      <v-card-subtitle>{{ dialogLabel }}</v-card-subtitle>
      <v-card-text>
        <p class="mb-3">
          {{ $t('botMembers.deleteText') }}
        </p>
        <v-alert
          v-if="(target?.balance ?? 0) > 0"
          type="warning"
          variant="tonal"
          density="compact"
          class="mb-2"
        >
          {{ $t('botMembers.deleteBalance', { n: fmtNum(target?.balance ?? 0) }) }}
        </v-alert>
        <template v-if="(target?.clients.length ?? 0) > 0">
          <v-checkbox
            v-model="deleteDialog.deleteClients"
            :label="$t('botMembers.deleteClients', { n: target?.clients.length ?? 0 })"
            color="error"
            hide-details
          />
          <p class="text-caption">
            {{ deleteDialog.deleteClients ? (target?.clients ?? []).join(', ') : $t('botMembers.deleteClientsOff') }}
          </p>
        </template>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="deleteDialog.visible = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="error"
          variant="flat"
          prepend-icon="mdi-delete"
          :loading="saving"
          @click="deleteCustomer"
        >
          {{ $t('actions.del') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-row>
    <v-col
      v-for="s in summary"
      :key="s.key"
      cols="6"
      sm="4"
      md="2"
    >
      <v-card
        rounded="xl"
        elevation="3"
        class="text-center pa-3"
      >
        <div class="text-h6">
          {{ s.value }}
        </div>
        <div class="text-caption">
          {{ $t('botMembers.sum.' + s.key) }}
        </div>
      </v-card>
    </v-col>
  </v-row>
  <v-row>
    <v-col
      cols="12"
      class="d-flex flex-wrap ga-2"
    >
      <v-btn
        color="primary"
        prepend-icon="mdi-send"
        :disabled="msgTargetsDefault.length == 0"
        @click="openMessage()"
      >
        {{ selected.length ? $t('botMembers.messageSelected', { n: msgTargetsDefault.length }) : $t('botMembers.messageShown', { n: msgTargetsDefault.length }) }}
      </v-btn>
      <v-btn
        prepend-icon="mdi-file-delimited"
        variant="tonal"
        :disabled="shown.length == 0"
        @click="exportCSV"
      >
        {{ $t('botMembers.export') }}
      </v-btn>
      <v-btn
        prepend-icon="mdi-chart-bar"
        variant="tonal"
        :active="showRefStats"
        @click="showRefStats = !showRefStats"
      >
        {{ $t('botMembers.refStats') }}
      </v-btn>
      <v-btn
        prepend-icon="mdi-refresh"
        variant="text"
        @click="load"
      >
        {{ $t('botMembers.reload') }}
      </v-btn>
    </v-col>
  </v-row>
  <v-expand-transition>
    <v-row v-if="showRefStats">
      <v-col cols="12">
        <v-card
          rounded="xl"
          elevation="3"
          :title="$t('botMembers.refStats')"
        >
          <v-data-table
            :items="refRows"
            :headers="refHeaders"
            :mobile="smAndDown"
            density="compact"
            items-per-page="-1"
            hide-default-footer
          >
            <template #item.ref="{ item }">
              <a
                href="#"
                @click.prevent="search = item.ref; filter = 'all'"
              >{{ item.ref || $t('botMembers.noRef') }}</a>
            </template>
            <template #item.conversion="{ item }">
              {{ item.members ? Math.round(item.buyers * 100 / item.members) + '%' : '-' }}
            </template>
            <template #item.spent="{ item }">
              {{ fmtNum(item.spent) }}
            </template>
          </v-data-table>
        </v-card>
      </v-col>
    </v-row>
  </v-expand-transition>
  <v-row>
    <v-col
      cols="12"
      md="5"
    >
      <v-text-field
        v-model="search"
        prepend-inner-icon="mdi-magnify"
        :label="$t('botMembers.search')"
        density="compact"
        hide-details
        clearable
      />
    </v-col>
    <v-col
      cols="12"
      md="7"
    >
      <v-chip-group
        v-model="filter"
        mandatory
        selected-class="text-primary"
      >
        <v-chip
          v-for="f in filters"
          :key="f"
          :value="f"
          filter
          variant="outlined"
        >
          {{ $t('botMembers.filter.' + f) }} ({{ counts[f] }})
        </v-chip>
      </v-chip-group>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12">
      <v-card
        rounded="xl"
        elevation="5"
      >
        <v-data-table
          v-model="selected"
          :items="shown"
          :headers="headers"
          :loading="loading"
          :mobile="smAndDown"
          item-value="tgId"
          show-select
          items-per-page="25"
          density="compact"
        >
          <template #item.user="{ item }">
            <div class="py-1">
              <div>
                <b>{{ item.name || '-' }}</b>
                <v-chip
                  v-if="item.blocked"
                  color="error"
                  size="x-small"
                  class="ms-1"
                >
                  {{ $t('botMembers.blocked') }}
                </v-chip>
              </div>
              <div class="text-caption">
                <bdi v-if="item.username"><a
                  :href="'https://t.me/' + item.username"
                  target="_blank"
                  rel="noopener"
                >@{{ item.username }}</a></bdi>
                <span v-if="item.username"> · </span>
                <bdi>{{ item.tgId }}</bdi>
              </div>
            </div>
          </template>
          <template #item.ref="{ item }">
            <span class="text-no-wrap">
              <span v-if="item.ref">{{ item.ref }}<span
                v-if="item.refGroup"
                class="text-caption"
              > ({{ item.refGroup }})</span></span>
              <span v-else>-</span>
              <v-btn
                icon="mdi-pencil"
                size="x-small"
                variant="text"
                @click="openRef(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.editRef')"
                />
              </v-btn>
            </span>
          </template>
          <template #item.seller="{ item }">
            <span class="text-no-wrap">
              {{ item.adminId ? sellerName(item.adminId) : '-' }}
              <v-btn
                icon="mdi-pencil"
                size="x-small"
                variant="text"
                @click="openSeller(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.editSeller')"
                />
              </v-btn>
            </span>
          </template>
          <template #item.firstSeen="{ item }">
            {{ fmtDate(item.firstSeen) }}
          </template>
          <template #item.trialAt="{ item }">
            <v-chip
              v-if="item.trialAt"
              color="info"
              size="small"
            >
              {{ fmtDate(item.trialAt) }}
            </v-chip>
            <span v-else>-</span>
          </template>
          <template #item.purchases="{ item }">
            <b>{{ purchases(item) }}</b>
            <span
              v-if="purchases(item) > 0"
              class="text-caption"
            > ({{ $t('botMembers.buyRenew', { buys: item.buys, renews: item.renews }) }})</span>
            <v-chip
              v-if="item.pending"
              color="warning"
              size="x-small"
              class="ms-1"
            >
              {{ $t('botMembers.pending', { n: item.pending }) }}
            </v-chip>
          </template>
          <template #item.spent="{ item }">
            {{ item.spent ? fmtNum(item.spent) : '-' }}
          </template>
          <template #item.balance="{ item }">
            {{ item.balance ? fmtNum(item.balance) : '-' }}
          </template>
          <template #item.lastBuy="{ item }">
            {{ fmtDate(item.lastBuy) }}
          </template>
          <template #item.clients="{ item }">
            <v-chip
              v-for="c in item.clients"
              :key="c"
              size="x-small"
              class="me-1"
            >
              {{ c }}
            </v-chip>
            <span v-if="item.clients.length == 0">-</span>
          </template>
          <template #item.actions="{ item }">
            <span class="text-no-wrap">
              <v-btn
                icon="mdi-account-edit"
                size="small"
                variant="text"
                @click="openEdit(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.editCustomer')"
                />
              </v-btn>
              <v-btn
                v-if="item.trialAt"
                icon="mdi-restore"
                color="warning"
                size="small"
                variant="text"
                @click="openTrial(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.trialReset')"
                />
              </v-btn>
              <v-btn
                icon="mdi-message-text"
                size="small"
                variant="text"
                :disabled="item.blocked"
                @click="openMessage([item.tgId])"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.message')"
                />
              </v-btn>
              <v-btn
                icon="mdi-wallet"
                size="small"
                variant="text"
                @click="openWallet(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.wallet')"
                />
              </v-btn>
              <v-btn
                :icon="item.blocked ? 'mdi-account-check' : 'mdi-account-cancel'"
                :color="item.blocked ? 'success' : 'error'"
                size="small"
                variant="text"
                @click="toggleBlock(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="item.blocked ? $t('botMembers.unblock') : $t('botMembers.block')"
                />
              </v-btn>
              <v-btn
                icon="mdi-delete"
                color="error"
                size="small"
                variant="text"
                @click="openDelete(item)"
              >
                <v-icon />
                <v-tooltip
                  activator="parent"
                  location="top"
                  :text="$t('botMembers.deleteCustomer')"
                />
              </v-btn>
            </span>
          </template>
        </v-data-table>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { BotMember, membersCSV, purchases, refStats, saveText } from '@/plugins/botMembers'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { computed, inject, onMounted, ref, Ref } from 'vue'
import { useDisplay } from 'vuetify'

interface RefOption {
  word: string
  group: string
}

interface PanelClient {
  id: number
  name: string
  tgId: number
}

interface Seller {
  id: number
  name: string
  owner: boolean
}

type Filter = 'all' | 'trial' | 'buyers' | 'trialOnly' | 'never' | 'blocked'

const loading: Ref<boolean> = inject('loading') ?? ref(false)
const { smAndDown } = useDisplay()
const t = (key: string, args: Record<string, unknown> = {}) => i18n.global.t(key, args)

const members = ref<BotMember[]>([])
const refs = ref<RefOption[]>([])
const sellers = ref<Seller[]>([])
const allClients = ref<PanelClient[]>([])
const search = ref('')
const filter = ref<Filter>('all')
const filters: Filter[] = ['all', 'trial', 'buyers', 'trialOnly', 'never', 'blocked']
const selected = ref<number[]>([])
const showRefStats = ref(false)

const bought = (m: BotMember) => purchases(m) > 0
const matches: Record<Filter, (m: BotMember) => boolean> = {
  all: () => true,
  trial: m => m.trialAt > 0,
  buyers: m => bought(m),
  trialOnly: m => m.trialAt > 0 && !bought(m),
  never: m => !bought(m),
  blocked: m => m.blocked,
}

const counts = computed(() => {
  const out = {} as Record<Filter, number>
  for (const f of filters) out[f] = members.value.filter(matches[f]).length
  return out
})

const fmtNum = (n: number): string => (n ?? 0).toLocaleString('en-US')

const summary = computed(() => {
  const trial = counts.value.trial
  const converted = members.value.filter(m => m.trialAt > 0 && bought(m)).length
  return [
    { key: 'members', value: fmtNum(members.value.length) },
    { key: 'trial', value: fmtNum(trial) },
    { key: 'buyers', value: fmtNum(counts.value.buyers) },
    { key: 'conversion', value: trial ? Math.round(converted * 100 / trial) + '%' : '-' },
    { key: 'sales', value: fmtNum(members.value.reduce((n, m) => n + purchases(m), 0)) },
    { key: 'spent', value: fmtNum(members.value.reduce((n, m) => n + m.spent, 0)) },
  ]
})

const sellerName = (id: number): string => {
  const s = sellers.value.find(x => x.id == id)
  const name = s?.name || String(id)
  return s?.owner ? `${t('botMembers.owner')} ${name}` : name
}

const shown = computed(() => {
  const q = (search.value ?? '').trim().toLowerCase().replace(/^@/, '')
  return members.value.filter(m => {
    if (!matches[filter.value](m)) return false
    if (q == '') return true
    return [m.name, m.username, String(m.tgId), m.ref, m.note, m.adminId ? sellerName(m.adminId) : '', ...m.clients]
      .some(v => (v ?? '').toLowerCase().includes(q))
  })
})

const headers = computed(() => [
  { title: t('botMembers.user'), key: 'user', sortable: false },
  { title: t('botMembers.ref'), key: 'ref' },
  { title: t('botMembers.seller'), key: 'seller', value: (m: BotMember) => m.adminId ? sellerName(m.adminId) : '' },
  { title: t('botMembers.joined'), key: 'firstSeen' },
  { title: t('botMembers.trial'), key: 'trialAt' },
  { title: t('botMembers.purchases'), key: 'purchases', value: (m: BotMember) => purchases(m) },
  { title: t('botMembers.spent'), key: 'spent' },
  { title: t('botMembers.balance'), key: 'balance' },
  { title: t('botMembers.lastBuy'), key: 'lastBuy' },
  { title: t('botMembers.clients'), key: 'clients', sortable: false },
  { title: '', key: 'actions', sortable: false },
])

const refRows = computed(() => refStats(members.value))
const refHeaders = computed(() => [
  { title: t('botMembers.ref'), key: 'ref' },
  { title: t('botMembers.sum.members'), key: 'members' },
  { title: t('botMembers.sum.trial'), key: 'trial' },
  { title: t('botMembers.sum.buyers'), key: 'buyers' },
  { title: t('botMembers.refConversion'), key: 'conversion', value: (r: { buyers: number, members: number }) => r.members ? r.buyers / r.members : 0 },
  { title: t('botMembers.sum.sales'), key: 'purchases' },
  { title: t('botMembers.sum.spent'), key: 'spent' },
])

const refItems = computed(() => refs.value.map(r => ({
  title: r.group ? `${r.word} (${r.group})` : r.word,
  value: r.word,
})))

const sellerItems = computed(() => [
  { title: t('botMembers.noSeller'), value: 0 },
  ...sellers.value.map(s => ({ title: sellerName(s.id), value: s.id })),
])

const fmtDate = (sec: number): string => {
  if (!sec) return '-'
  const locale = i18n.global.locale.value.replace('zh', 'zh-')
  return new Date(sec * 1000).toLocaleDateString(locale)
}

const load = async () => {
  loading.value = true
  const msg = await HttpUtils.get<{ members: BotMember[], refs: RefOption[], sellers: Seller[], clients: PanelClient[] }>('api/botMembers')
  loading.value = false
  if (msg.success && msg.obj) {
    members.value = msg.obj.members ?? []
    refs.value = msg.obj.refs ?? []
    sellers.value = msg.obj.sellers ?? []
    allClients.value = msg.obj.clients ?? []
    const known = new Set(members.value.map(m => m.tgId))
    selected.value = selected.value.filter(id => known.has(id))
  }
}

onMounted(load)

// The member the open dialog is about.
const target = ref<BotMember | null>(null)
const dialogLabel = computed(() => {
  const m = target.value
  if (!m) return ''
  return [m.name, m.username ? '@' + m.username : '', String(m.tgId)].filter(s => s).join(' ')
})
const saving = ref(false)

const refDialog = ref({ visible: false, current: '', word: '' })
const openRef = (m: BotMember) => {
  target.value = m
  refDialog.value = { visible: true, current: m.ref, word: m.ref }
}

const sellerDialog = ref({ visible: false, current: 0, seller: 0 })
const openSeller = (m: BotMember) => {
  target.value = m
  sellerDialog.value = { visible: true, current: m.adminId, seller: m.adminId }
}

const walletDialog = ref({ visible: false, balance: 0, sign: '+', amount: 0, note: '', notify: true })
const openWallet = (m: BotMember) => {
  target.value = m
  walletDialog.value = { visible: true, balance: m.balance, sign: '+', amount: 0, note: '', notify: true }
}

// act posts one change of the member the open dialog is about, then closes
// the dialogs and reloads the list.
const act = async (action: string, data: Record<string, string | number | boolean>): Promise<boolean> => {
  if (!target.value) return false
  saving.value = true
  const msg = await HttpUtils.post(`api/${action}`, { tgId: target.value.tgId, ...data })
  saving.value = false
  if (!msg.success) return false
  refDialog.value.visible = false
  sellerDialog.value.visible = false
  walletDialog.value.visible = false
  await load()
  return true
}

const post = async <T = unknown>(action: string, data: Record<string, string | number | boolean>) => {
  return HttpUtils.post<T>(`api/${action}`, { tgId: target.value?.tgId ?? 0, ...data })
}

// The edit dialog: note, bound configs, introducer, seller and blocking.
const editDialog = ref({ visible: false, note: '', clients: [] as number[], ref: '', seller: 0, blocked: false })
const clientItems = computed(() => {
  const me = target.value?.tgId ?? 0
  return allClients.value.map(c => ({
    title: c.tgId && c.tgId != me ? `${c.name} (${t('botMembers.boundTo', { id: c.tgId })})` : c.name,
    value: c.id,
  }))
})
const openEdit = (m: BotMember) => {
  target.value = m
  editDialog.value = { visible: true, note: m.note ?? '', clients: [...(m.clientIds ?? [])], ref: m.ref, seller: m.adminId, blocked: m.blocked }
}
const sameIds = (a: number[], b: number[]) => a.length == b.length && [...a].sort().join(',') == [...b].sort().join(',')
const saveEdit = async () => {
  const m = target.value
  if (!m) return
  const d = editDialog.value
  saving.value = true
  let ok = true
  const clientsChanged = !sameIds(d.clients, m.clientIds ?? [])
  if (d.note.trim() != (m.note ?? '').trim() || clientsChanged) {
    ok = (await post('botMemberEdit', { note: d.note, setClients: clientsChanged, clients: d.clients.join(',') })).success
  }
  if (ok && d.ref != m.ref) ok = (await post('botMemberRef', { word: d.ref })).success
  if (ok && d.seller != m.adminId) ok = (await post('botMemberSeller', { seller: d.seller })).success
  if (ok && d.blocked != m.blocked) ok = (await post('botMemberBlock', { blocked: d.blocked })).success
  saving.value = false
  if (ok) {
    editDialog.value.visible = false
    push.success({ title: t('botMembers.edited') })
  }
  await load()
}

// The trial reset dialog.
const trialDialog = ref({ visible: false, names: [] as string[], ids: [] as number[], deleteClient: true, notify: true })
const openTrial = (m: BotMember) => {
  target.value = m
  const ids = (m.trialClients ?? []).filter(id => allClients.value.some(c => c.id == id))
  const names = ids.map(id => allClients.value.find(c => c.id == id)?.name ?? String(id))
  trialDialog.value = { visible: true, names, ids, deleteClient: true, notify: true }
}
// How many configs the customer keeps after the reset.
const trialStillBound = computed(() => {
  const m = target.value
  if (!m) return 0
  const d = trialDialog.value
  const gone = d.deleteClient ? new Set(d.ids) : new Set<number>()
  return (m.clientIds ?? []).filter(id => !gone.has(id)).length
})
const resetTrial = async () => {
  const d = trialDialog.value
  saving.value = true
  const msg = await post<{ orders: number, deleted: string[], bound: number }>('botMemberTrialReset', { deleteClient: d.deleteClient && d.ids.length > 0, notify: d.notify })
  saving.value = false
  if (msg.success) {
    trialDialog.value.visible = false
    push.success({ title: t('botMembers.trialDone') })
    if ((msg.obj?.bound ?? 0) > 0) push.warning({ title: t('botMembers.trialStillBound', { n: msg.obj?.bound ?? 0 }) })
  }
  await load()
}

// The delete dialog.
const deleteDialog = ref({ visible: false, deleteClients: false })
const openDelete = (m: BotMember) => {
  target.value = m
  deleteDialog.value = { visible: true, deleteClients: false }
}
const deleteCustomer = async () => {
  saving.value = true
  const msg = await post('botMemberDelete', { deleteClients: deleteDialog.value.deleteClients })
  saving.value = false
  if (msg.success) {
    deleteDialog.value.visible = false
    push.success({ title: t('botMembers.deleted') })
  }
  await load()
}

const saveWallet = async () => {
  const w = walletDialog.value
  const amount = Math.floor(Number(w.amount))
  if (!(amount > 0)) return
  await act('botMemberWallet', { delta: w.sign == '-' ? -amount : amount, note: w.note, notify: w.notify })
}

const toggleBlock = async (m: BotMember) => {
  if (!m.blocked && !confirm(t('botMembers.blockConfirm', { name: m.name || m.username || m.tgId }))) return
  target.value = m
  await act('botMemberBlock', { blocked: !m.blocked })
}

// Who a message goes to: the ticked members, else everybody on screen;
// blocked members never.
const msgTargetsDefault = computed(() => {
  const ids = selected.value.length ? selected.value : shown.value.map(m => m.tgId)
  const blocked = new Set(members.value.filter(m => m.blocked).map(m => m.tgId))
  return ids.filter(id => !blocked.has(id))
})
const msgDialog = ref({ visible: false, text: '', ids: [] as number[] })
const msgTargets = computed(() => msgDialog.value.ids)
const openMessage = (ids?: number[]) => {
  msgDialog.value = { visible: true, text: msgDialog.value.text, ids: ids ?? [...msgTargetsDefault.value] }
}
const sendMessage = async () => {
  saving.value = true
  const msg = await HttpUtils.post<{ queued: number }>('api/botMembersMessage', { ids: msgTargets.value.join(','), text: msgDialog.value.text })
  saving.value = false
  if (msg.success) {
    push.success({ title: t('botMembers.sent', { n: msg.obj?.queued ?? 0 }) })
    msgDialog.value = { visible: false, text: '', ids: [] }
  }
}

const exportCSV = () => {
  const date = new Date().toISOString().slice(0, 10)
  saveText(`bot-members-${date}.csv`, membersCSV(shown.value, sellerName))
}
</script>
