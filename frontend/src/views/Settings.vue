<template>
  <v-card :loading="loading">
    <v-tabs
      v-model="tab"
      color="primary"
      align-tabs="center"
      show-arrows
    >
      <v-tab value="t1">
        {{ $t('setting.interface') }}
      </v-tab>
      <v-tab value="t2">
        {{ $t('setting.sub') }}
      </v-tab>
      <v-tab value="t3">
        {{ $t('setting.jsonSub') }}
      </v-tab>
      <v-tab value="t4">
        {{ $t('setting.clashSub') }}
      </v-tab>
      <v-tab value="t7">
        {{ $t('panelUpdate.tab') }}
        <v-badge
          v-if="updateAvailable"
          dot
          inline
          color="info"
        />
      </v-tab>
    </v-tabs>
    <v-card-text>
      <v-row
        align="center"
        justify="center"
        style="margin-bottom: 10px;"
      >
        <v-col cols="auto">
          <v-btn
            color="primary"
            :loading="loading"
            :disabled="!stateChange"
            @click="save"
          >
            {{ $t('actions.save') }}
          </v-btn>
        </v-col>
        <v-col cols="auto">
          <v-btn
            variant="outlined"
            color="warning"
            :loading="loading"
            :disabled="stateChange"
            @click="restartApp"
          >
            {{ $t('actions.restartApp') }}
          </v-btn>
        </v-col>
        <!-- The core cannot be held down any other way: a plain stop is undone by
           the watchdog within five seconds. -->
        <v-col cols="auto">
          <v-btn
            variant="outlined"
            :color="maintenance ? 'success' : 'error'"
            :loading="loading"
            :disabled="stateChange"
            @click="toggleMaintenance"
          >
            {{ maintenance ? $t('actions.startCore') : $t('actions.stopCore') }}
          </v-btn>
        </v-col>
      </v-row>
      <v-alert
        v-if="maintenance"
        type="warning"
        variant="tonal"
        density="compact"
        class="mb-4"
        :text="$t('setting.maintenanceOnHint')"
      />
      <v-window v-model="tab">
        <v-window-item value="t1">
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webListen"
                :label="$t('setting.addr')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="webPort"
                min="1"
                type="number"
                :label="$t('setting.port')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webPath"
                :label="$t('setting.webPath')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webDomain"
                :label="$t('setting.domain')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webKeyFile"
                :label="$t('setting.sslKey')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webCertFile"
                :label="$t('setting.sslCert')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webURI"
                :label="$t('setting.webUri')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webDecoyDir"
                :label="$t('setting.decoyDir')"
                :hint="$t('setting.decoyDirHint')"
                persistent-hint
                dir="ltr"
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="sessionMaxAge"
                type="number"
                min="0"
                :label="$t('setting.sessionAge')"
                :suffix="$t('date.m')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="trafficAge"
                type="number"
                min="0"
                :label="$t('setting.trafficAge')"
                :suffix="$t('date.d')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="statsBucketSeconds"
                v-tooltip:top="$t('setting.statsBucketSecondsHint')"
                type="number"
                min="1"
                :label="$t('setting.statsBucketSeconds')"
                :suffix="$t('date.s')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.timeLocation"
                :label="$t('setting.timeLoc')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.globalReset"
                v-tooltip:top="$t('setting.globalResetHint')"
                :label="$t('setting.globalReset')"
                hide-details
                placeholder="0 0 1 * *"
              />
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t2">
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subEncode"
                color="primary"
                :label="$t('setting.subEncode')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subShowInfo"
                color="primary"
                :label="$t('setting.subInfo')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subPage"
                color="primary"
                :label="$t('setting.subPage')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subLoadOrder"
                color="primary"
                :label="$t('setting.subLoadOrder')"
                :disabled="subNodeRoute.enable"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row v-if="subPage">
            <v-col cols="12">
              <v-expansion-panels variant="accordion">
                <v-expansion-panel>
                  <v-expansion-panel-title>
                    <v-icon
                      icon="mdi-card-account-details-outline"
                      class="me-2"
                    />
                    {{ $t('setting.subPageLook') }}
                  </v-expansion-panel-title>
                  <v-expansion-panel-text>
                    <div class="text-caption text-medium-emphasis mb-3">
                      {{ $t('setting.subPageLookHint') }}
                    </div>
                    <v-row>
                      <v-col
                        v-for="k in subPageTextFields"
                        :key="k"
                        cols="12"
                        sm="6"
                        md="4"
                      >
                        <v-text-field
                          :model-value="subPageConf[k]"
                          :label="$t('setting.subPage_' + k)"
                          :placeholder="subPagePlaceholder[k]"
                          :dir="k === 'name' ? undefined : 'ltr'"
                          density="compact"
                          hide-details
                          @update:model-value="(v: string) => setSubPage(k, v)"
                        >
                          <template
                            v-if="k === 'accent' && /^#[0-9a-fA-F]{3,6}$/.test(subPageConf.accent)"
                            #append-inner
                          >
                            <span :style="{ background: subPageConf.accent, width: '18px', height: '18px', borderRadius: '50%', display: 'inline-block' }" />
                          </template>
                        </v-text-field>
                      </v-col>
                    </v-row>
                    <v-row>
                      <v-col
                        v-for="k in (['notice', 'noticeEn'] as const)"
                        :key="k"
                        cols="12"
                        md="6"
                      >
                        <v-textarea
                          :model-value="subPageConf[k]"
                          :label="$t('setting.subPage_' + k)"
                          :dir="k === 'noticeEn' ? 'ltr' : undefined"
                          rows="2"
                          auto-grow
                          density="compact"
                          hide-details
                          @update:model-value="(v: string) => setSubPage(k, v)"
                        />
                      </v-col>
                    </v-row>
                    <v-row dense>
                      <v-col
                        v-for="k in subPageFlagKeys"
                        :key="k"
                        cols="12"
                        sm="6"
                        md="4"
                      >
                        <v-switch
                          :model-value="subPageConf[k]"
                          :label="$t('setting.subPage_' + k)"
                          color="primary"
                          density="compact"
                          hide-details
                          @update:model-value="(v: boolean | null) => setSubPage(k, !!v)"
                        />
                      </v-col>
                    </v-row>
                  </v-expansion-panel-text>
                </v-expansion-panel>
              </v-expansion-panels>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12">
              <v-switch
                :model-value="subNodeRoute.enable"
                color="primary"
                :label="$t('setting.subNodeRoute')"
                :hint="$t('setting.subNodeRouteHint')"
                persistent-hint
                @update:model-value="setRouteEnabled"
              />
            </v-col>
            <v-col
              v-if="subNodeRoute.enable"
              cols="12"
            >
              <div
                v-for="(row, idx) in subNodeRoute.entries"
                :key="row.id"
                class="d-flex align-center ga-2 mb-2"
              >
                <v-btn
                  icon="mdi-arrow-up"
                  variant="text"
                  size="small"
                  :disabled="idx === 0"
                  :aria-label="$t('setting.subNodeRouteMoveUp')"
                  @click="moveRouteEntry(idx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  variant="text"
                  size="small"
                  :disabled="idx === subNodeRoute.entries.length - 1"
                  :aria-label="$t('setting.subNodeRouteMoveDown')"
                  @click="moveRouteEntry(idx, 1)"
                />
                <div class="flex-grow-1">
                  {{ routeNodeName(row.id) }}
                  <span class="text-medium-emphasis ml-2">{{ routeShareLabel(row) }}</span>
                </div>
                <v-text-field
                  :model-value="row.weight"
                  type="number"
                  min="0"
                  max="10000"
                  hide-details
                  density="compact"
                  style="max-width: 8rem"
                  :label="$t('setting.subNodeRouteWeight')"
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
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subListen"
                :label="$t('setting.addr')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="subPort"
                type="number"
                min="1"
                :label="$t('setting.port')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subKeyFile"
                :label="$t('setting.sslKey')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subCertFile"
                :label="$t('setting.sslCert')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subDomain"
                :label="$t('setting.domain')"
                :hint="$t('setting.subDomainHint')"
                persistent-hint
                dir="ltr"
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subPath"
                :label="$t('setting.path')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="subUpdates"
                type="number"
                min="0"
                :label="$t('setting.update')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subURI"
                :label="$t('setting.subUri')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              md="8"
            >
              <v-textarea
                v-model="settings.subCustomHeaders"
                v-tooltip:top="$t('setting.subCustomHeadersHint')"
                :label="$t('setting.subCustomHeaders')"
                :placeholder="'announce: Hello!\nannounce-url: https://example.com/'"
                auto-grow
                rows="3"
                hide-details
              />
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t3">
          <SubJsonExtVue :settings="settings" />
        </v-window-item>

        <v-window-item value="t4">
          <SubClashExtVue :settings="settings" />
        </v-window-item>

        <!-- eager: it looks for a new release as soon as the page opens, so the tab
           can show that one is out. -->
        <v-window-item
          value="t7"
          eager
        >
          <PanelUpdateVue @available="updateAvailable = $event" />
        </v-window-item>
      </v-window>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { i18n } from '@/locales'
import { Ref, computed, inject, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { FindDiff } from '@/plugins/utils'
import { parseSubPageConf, subPageConfJSON, subPageFlagKeys, type SubPageConf } from '@/plugins/subPageConf'
import SubJsonExtVue from '@/components/SubJsonExt.vue'
import SubClashExtVue from '@/components/SubClashExt.vue'
import PanelUpdateVue from '@/components/PanelUpdate.vue'
import { push } from 'notivue'
import Data from '@/store/modules/data'
// The Update tab reloads the page into the new version once an update is
// done, and marks that it wants to be open again.
const tab = ref(sessionStorage.getItem('panelUpdate.reopen') ? 't7' : 't1')
sessionStorage.removeItem('panelUpdate.reopen')
// A release newer than this panel is out (the Update tab shows a dot).
const updateAvailable = ref(false)
const loading:Ref = inject('loading')?? ref(false)
const oldSettings = ref<Record<string, string>>({})

const settings = ref({
	webListen: "",
	webDomain: "",
	webPort: "2095",
	webCertFile: "",
	webKeyFile: "",
  webPath: "/app/",
  webURI: "",
	sessionMaxAge: "0",
  trafficAge: "30",
  statsBucketSeconds: "60",
	timeLocation: "Asia/Tehran",
  subListen: "",
	subPort: "2096",
	subPath: "/sub/",
	subDomain: "",
	subCertFile: "",
	subKeyFile: "",
	subUpdates: "12",
	subEncode: "true",
	subShowInfo: "false",
	subPage: "true",
	subPageConf: "{}",
	subLoadOrder: "false",
	subNodeRoute: "{}",
	webDecoyDir: "",
	subURI: "",
	subCustomHeaders: "",
  subJsonExt: "",
  subClashExt: "",
  subClashNoDefGrp: "false",
  subClashSprtAll: "false",
  subClashUdp: "false",
  globalReset: "",
  tgBotEnable: "false",
  tgBotToken: "",
  tgBotAdmins: "",
  tgBotOwner: "",
  tgBotScopes: "",
  tgBotPerms: "",
  tgBotProxy: "",
  tgBotOutbound: "",
  tgBotLang: "fa",
  tgBotSkin: "colorful",
  tgBotNotify: "true",
  tgBotPanel: "false",
  tgBotReport: "",
  tgBotReportBackup: "false",
  tgBotShop: "{}",
})

// The panel settings, exactly as the block above spells them out.
type PanelSettings = typeof settings.value

onMounted(async () => {
  loading.value = true
  await Promise.all([loadData(), Data().loadData()])
  loading.value = false
})

const loadData = async () => {
  loading.value = true
  const msg = await HttpUtils.get<PanelSettings>('api/settings')
  loading.value = false
  if (msg.success) {
    setData(msg.obj)
  }
}

const setData = (data: PanelSettings) => {
  settings.value = data
  oldSettings.value = { ...data }
}

const save = async () => {
  loading.value = true
  // Only what changed is sent, so a value the Telegram bot page (or the bot)
  // saved since this page was opened is not put back.
  const changed: Record<string, string> = {}
  for (const [k, v] of Object.entries(settings.value)) {
    if (oldSettings.value[k] !== v) changed[k] = v
  }
  const msg = await HttpUtils.post<{ settings: PanelSettings }>('api/save', { object: 'settings', action: 'set', data: JSON.stringify(changed) })
  if (msg.success) {
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t('actions.set') + " " + i18n.global.t('pages.settings')
    })
    setData(msg.obj.settings)
  }
  loading.value = false
}

const maintenance = computed((): boolean => Data().maintenance)

// Stopping the core is deliberate downtime for every user, so it is confirmed
// rather than done on a single click.
const toggleMaintenance = async () => {
  const turningOn = !maintenance.value
  if (turningOn && !confirm(i18n.global.t('setting.maintenanceConfirm'))) return

  loading.value = true
  const msg = await HttpUtils.post('api/maintenance', { enable: turningOn })
  if (msg.success) {
    Data().maintenance = turningOn
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t(turningOn ? 'setting.maintenanceOn' : 'setting.maintenanceOff')
    })
  }
  loading.value = false
}

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

const restartApp = async () => {
  loading.value = true
  const msg = await HttpUtils.post('api/restartApp',{})
  if (msg.success) {
    let url = settings.value.webURI
    if (url !== "") {
      const isTLS = settings.value.webCertFile !== "" || settings.value.webKeyFile !== ""
      url = buildURL(settings.value.webDomain,settings.value.webPort.toString(),isTLS, settings.value.webPath)
    }
    await sleep(3000)
    window.location.replace(url)
  }
  loading.value = false
}

const buildURL = (host: string, port: string, isTLS: boolean, path: string) => {
  if (!host || host.length == 0) host = window.location.hostname
  if (!port || port.length == 0) port = window.location.port

  const protocol = isTLS ? "https:" : "http:"

  if (port === "" || (isTLS && port === "443") || (!isTLS && port === "80")) {
      port = ""
  } else {
      port = `:${port}`
  }

  return `${protocol}//${host}${port}${path}settings`
}

const subEncode = computed({
  get: () => { return settings.value.subEncode == "true" },
  set: (v:boolean) => { settings.value.subEncode = v ? "true" : "false" }
})

const subLoadOrder = computed({
  get: () => { return settings.value.subLoadOrder == "true" },
  set: (v:boolean) => { settings.value.subLoadOrder = v ? "true" : "false" }
})

type NodeRouteEntry = { id: number, weight: number }
type NodeRoute = { enable: boolean, entries: NodeRouteEntry[] }

const parseNodeRoute = (raw: string): NodeRoute => {
  try {
    const o = JSON.parse(raw || '{}')
    const entries = Array.isArray(o.entries) ? o.entries : []
    return {
      enable: !!o.enable,
      entries: entries
        .map((e: { id?: number, weight?: number }) => ({
          id: Number(e?.id),
          weight: Math.max(0, Math.min(10000, Math.trunc(Number(e?.weight) || 0))),
        }))
        .filter((e: NodeRouteEntry) => e.id > 0),
    }
  } catch {
    return { enable: false, entries: [] }
  }
}

const subNodeRoute = computed({
  get: () => parseNodeRoute(settings.value.subNodeRoute || '{}'),
  set: (v: NodeRoute) => {
    settings.value.subNodeRoute = JSON.stringify({
      enable: v.enable,
      entries: v.entries.map(e => ({
        id: e.id,
        weight: Math.max(0, Math.min(10000, Math.trunc(Number(e.weight) || 0))),
      })),
    })
  },
})

const setRouteEnabled = (v: boolean | null) => {
  const cur = subNodeRoute.value
  let entries = cur.entries
  if (v && entries.length === 0) {
    entries = [...routeNodes.value]
      .sort((a, b) => (a.sortOrder ?? 0) - (b.sortOrder ?? 0) || a.name.localeCompare(b.name))
      .map(n => ({ id: n.id, weight: 100 }))
  }
  subNodeRoute.value = { enable: !!v, entries }
}

const setRouteWeight = (idx: number, v: string | number) => {
  const next = subNodeRoute.value.entries.map((e, i) => i === idx
    ? { ...e, weight: Math.max(0, Math.min(10000, Math.trunc(Number(v) || 0))) }
    : e)
  subNodeRoute.value = { ...subNodeRoute.value, entries: next }
}

const routeNodes = computed(() => Data().nodes ?? [])

const routeNodeName = (id: number) => {
  const n = routeNodes.value.find(x => x.id === id)
  return n?.name || ('#' + id)
}

const routeShareLabel = (row: NodeRouteEntry) => {
  const total = subNodeRoute.value.entries.reduce((s, e) => s + Math.max(0, Number(e.weight) || 0), 0)
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

const subPage = computed({
  get: () => { return settings.value.subPage != "false" },
  set: (v:boolean) => { settings.value.subPage = v ? "true" : "false" }
})

// The page's look: subPageConf, edited a field at a time.
const subPageConf = computed((): SubPageConf => parseSubPageConf(settings.value.subPageConf))
const subPageTextFields = ['name', 'nameEn', 'logo', 'accent', 'support', 'channel'] as const
const subPagePlaceholder: Record<string, string> = {
  name: 'وضعیت اشتراک', nameEn: 'Subscription status', logo: 'https://example.com/logo.png',
  accent: '#0C96B2', support: 'https://t.me/support', channel: 'https://t.me/channel',
}
const setSubPage = <K extends keyof SubPageConf>(k: K, v: SubPageConf[K]) => {
  settings.value.subPageConf = subPageConfJSON({ ...subPageConf.value, [k]: v })
}

const subShowInfo = computed({
  get: () => { return settings.value.subShowInfo == "true" },
  set: (v:boolean) => { settings.value.subShowInfo = v ? "true" : "false" }
})

const webPort = computed({
  get: () => { return settings.value.webPort.length>0 ? parseInt(settings.value.webPort) : 2095 },
  set: (v:number) => { settings.value.webPort = v>0 ? v.toString() : "2095" }
})

const sessionMaxAge = computed({
  get: () => { return settings.value.sessionMaxAge.length>0 ? parseInt(settings.value.sessionMaxAge) : 0 },
  set: (v:number) => { settings.value.sessionMaxAge = v>0 ? v.toString() : "0" }
})

const trafficAge = computed({
  get: () => { return settings.value.trafficAge.length>0 ? parseInt(settings.value.trafficAge) : 0 },
  set: (v:number) => { settings.value.trafficAge = v>0 ? v.toString() : "0" }
})

const statsBucketSeconds = computed({
  get: () => { return settings.value.statsBucketSeconds.length>0 ? parseInt(settings.value.statsBucketSeconds) : 60 },
  set: (v:number) => { settings.value.statsBucketSeconds = v>0 ? v.toString() : "60" }
})

const subPort = computed({
  get: () => { return settings.value.subPort.length>0 ? parseInt(settings.value.subPort) : 2096 },
  set: (v:number) => { settings.value.subPort = v>0 ? v.toString() : "2096" }
})

const subUpdates = computed({
  get: () => { return settings.value.subUpdates.length>0 ? parseInt(settings.value.subUpdates) : 12 },
  set: (v:number) => { settings.value.subUpdates = v>0 ? v.toString() : "12" }
})

const stateChange = computed(() => {
  return !FindDiff.deepCompare(settings.value,oldSettings.value)
})
</script>
