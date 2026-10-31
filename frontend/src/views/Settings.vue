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
                v-model="subRemarkPrefix"
                color="primary"
                :label="$t('setting.subRemarkPrefix')"
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
                    <div class="text-subtitle-2 mt-4 mb-1">
                      {{ $t('setting.subPageNotices') }}
                    </div>
                    <div class="text-caption text-medium-emphasis mb-2">
                      {{ $t('setting.subPageNoticesHint') }}
                    </div>
                    <v-card
                      v-for="(n, idx) in noticeRows"
                      :key="'n' + idx"
                      variant="outlined"
                      class="pa-3 mb-2"
                    >
                      <v-row dense>
                        <v-col
                          cols="12"
                          md="6"
                        >
                          <v-textarea
                            v-model="n.text"
                            :label="$t('setting.subPage_notice')"
                            rows="2"
                            auto-grow
                            density="compact"
                            hide-details
                            @update:model-value="writeSubPageLists"
                          />
                        </v-col>
                        <v-col
                          cols="12"
                          md="6"
                        >
                          <v-textarea
                            v-model="n.textEn"
                            :label="$t('setting.subPage_noticeEn')"
                            dir="ltr"
                            rows="2"
                            auto-grow
                            density="compact"
                            hide-details
                            @update:model-value="writeSubPageLists"
                          />
                        </v-col>
                        <v-col
                          cols="12"
                          sm="5"
                        >
                          <v-select
                            v-model="n.kind"
                            :items="subPageNoticeKinds.map(k => ({ value: k, title: $t('setting.subPageKind_' + k) }))"
                            :label="$t('setting.subPageNoticeKind')"
                            density="compact"
                            hide-details
                            @update:model-value="writeSubPageLists"
                          />
                        </v-col>
                        <v-col
                          cols="10"
                          sm="5"
                        >
                          <v-text-field
                            v-model="n.until"
                            type="date"
                            :label="$t('setting.subPageNoticeUntil')"
                            density="compact"
                            hide-details
                            clearable
                            @update:model-value="(v: string | null) => { n.until = v || ''; writeSubPageLists() }"
                          />
                        </v-col>
                        <v-col
                          cols="2"
                          class="d-flex align-center justify-end"
                        >
                          <v-btn
                            icon="mdi-delete-outline"
                            variant="text"
                            size="small"
                            :aria-label="$t('setting.subNodeRouteRemove')"
                            @click="noticeRows.splice(idx, 1); writeSubPageLists()"
                          />
                        </v-col>
                      </v-row>
                    </v-card>
                    <v-btn
                      v-if="noticeRows.length < 10"
                      size="small"
                      variant="tonal"
                      prepend-icon="mdi-plus"
                      class="mb-2"
                      @click="noticeRows.push({ text: '', textEn: '', kind: 'info', until: '' })"
                    >
                      {{ $t('setting.subPageNoticeAdd') }}
                    </v-btn>
                    <div class="text-subtitle-2 mt-4 mb-1">
                      {{ $t('setting.subPageSites') }}
                    </div>
                    <div class="text-caption text-medium-emphasis mb-2">
                      {{ $t('setting.subPageSitesHint') }}
                    </div>
                    <div
                      v-for="(x, idx) in siteRows"
                      :key="'s' + idx"
                      class="d-flex align-center ga-2 mb-2"
                    >
                      <v-text-field
                        v-model="x.name"
                        :label="$t('setting.subPageSiteName')"
                        density="compact"
                        hide-details
                        style="max-width: 12rem"
                        @update:model-value="writeSubPageLists"
                      />
                      <v-text-field
                        v-model="x.url"
                        :label="$t('setting.subPageSiteUrl')"
                        placeholder="https://www.google.com/generate_204"
                        dir="ltr"
                        density="compact"
                        hide-details
                        @update:model-value="writeSubPageLists"
                      />
                      <v-btn
                        icon="mdi-close"
                        variant="text"
                        size="small"
                        :aria-label="$t('setting.subNodeRouteRemove')"
                        @click="siteRows.splice(idx, 1); writeSubPageLists()"
                      />
                    </div>
                    <v-btn
                      v-if="siteRows.length < 24"
                      size="small"
                      variant="tonal"
                      prepend-icon="mdi-plus"
                      class="mb-4"
                      @click="siteRows.push({ name: '', url: 'https://' })"
                    >
                      {{ $t('setting.subPageSiteAdd') }}
                    </v-btn>
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
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                icon="mdi-scale-balance"
              >
                {{ $t('setting.balanceMoved') }}
                <router-link to="/nodes">
                  {{ $t('pages.nodes') }}
                </router-link>
              </v-alert>
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
import { Ref, computed, inject, onMounted, ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { FindDiff } from '@/plugins/utils'
import { parseSubPageConf, subPageConfJSON, subPageFlagKeys, subPageNoticeKinds, type SubPageConf, type SubPageNotice, type SubPageSite } from '@/plugins/subPageConf'
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
	subRemarkPrefix: "true",
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

const subPage = computed({
  get: () => { return settings.value.subPage != "false" },
  set: (v:boolean) => { settings.value.subPage = v ? "true" : "false" }
})

// The page's look: subPageConf, edited a field at a time.
const subPageConf = computed((): SubPageConf => parseSubPageConf(settings.value.subPageConf))
const subPageTextFields = ['name', 'nameEn', 'logo', 'accent', 'support', 'channel'] as const
const subPagePlaceholder: Record<string, string> = {
  name: 'وضعیت اشتراک', nameEn: 'Subscription status', logo: 'https://example.com/logo.png',
  accent: '#0C96B2', support: 'https://t.me/DrNetWorkSingBox?direct', channel: 'https://t.me/channel',
}
// The notice and site lists are edited here, blank rows and all; the setting
// gets them without the blank ones.
const noticeRows = ref<SubPageNotice[]>([])
const siteRows = ref<SubPageSite[]>([])
let subPageWritten = ''
watch(() => settings.value.subPageConf, (raw) => {
  if (raw === subPageWritten) return
  const c = parseSubPageConf(raw)
  noticeRows.value = c.notices
  siteRows.value = c.sites
}, { immediate: true })
const writeSubPage = (c: SubPageConf) => {
  subPageWritten = subPageConfJSON({ ...c, notices: noticeRows.value, sites: siteRows.value })
  settings.value.subPageConf = subPageWritten
}
const writeSubPageLists = () => writeSubPage(subPageConf.value)
const setSubPage = <K extends keyof SubPageConf>(k: K, v: SubPageConf[K]) => {
  writeSubPage({ ...subPageConf.value, [k]: v })
}

const subRemarkPrefix = computed({
  get: () => { return settings.value.subRemarkPrefix == "true" },
  set: (v:boolean) => { settings.value.subRemarkPrefix = v ? "true" : "false" }
})

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
