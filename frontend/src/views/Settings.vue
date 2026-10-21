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
      <v-tab value="t5">
        {{ $t('setting.tgBot') }}
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

        <v-window-item value="t5">
          <v-tabs
            v-model="tgTab"
            color="primary"
            density="compact"
            class="mb-3"
          >
            <v-tab value="bot">
              {{ $t('setting.tgBotTabBot') }}
            </v-tab>
            <v-tab value="shop">
              {{ $t('setting.tgBotTabShop') }}
            </v-tab>
            <v-tab value="desks">
              {{ $t('setting.tgBotTabDesks') }}
            </v-tab>
          </v-tabs>
          <v-window v-model="tgTab">
            <v-window-item value="bot">
              <v-row>
                <v-col cols="12">
                  <v-alert
                    type="info"
                    variant="tonal"
                    density="compact"
                  >
                    {{ $t('setting.tgBotHint') }}
                  </v-alert>
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="tgBotEnable"
                    color="primary"
                    :label="$t('setting.tgBotEnable')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="tgBotNotify"
                    color="primary"
                    :label="$t('setting.tgBotNotify')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-switch
                    v-model="tgBotPanel"
                    color="primary"
                    :label="$t('setting.tgBotPanel')"
                    :hint="$t('setting.tgBotPanelHint')"
                    persistent-hint
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-select
                    v-model="settings.tgBotLang"
                    :items="[{ title: 'فارسی', value: 'fa' }, { title: 'English', value: 'en' }]"
                    :label="$t('setting.tgBotLang')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-select
                    v-model="settings.tgBotSkin"
                    :items="[{ title: $t('setting.tgBotSkinColorful'), value: 'colorful' }, { title: $t('setting.tgBotSkinClassic'), value: 'classic' }]"
                    :label="$t('setting.tgBotSkin')"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-text-field
                    v-model="settings.tgBotToken"
                    :label="$t('setting.tgBotToken')"
                    placeholder="123456789:AA..."
                    autocomplete="off"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-text-field
                    v-model="settings.tgBotAdmins"
                    :label="$t('setting.tgBotAdmins')"
                    :hint="$t('setting.tgBotAdminsHint')"
                    persistent-hint
                    placeholder="12345678, 87654321"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-text-field
                    v-model="settings.tgBotOwner"
                    :label="$t('setting.tgBotOwner')"
                    :hint="$t('setting.tgBotOwnerHint')"
                    persistent-hint
                    placeholder="12345678"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-textarea
                    v-model="settings.tgBotScopes"
                    :label="$t('setting.tgBotScopes')"
                    :hint="$t('setting.tgBotScopesHint')"
                    persistent-hint
                    rows="2"
                    auto-grow
                    placeholder="12345678=Sales"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-textarea
                    v-model="settings.tgBotPerms"
                    :label="$t('setting.tgBotPerms')"
                    :hint="$t('setting.tgBotPermsHint')"
                    persistent-hint
                    rows="2"
                    auto-grow
                    placeholder="12345678=clients,stats"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-text-field
                    v-model="settings.tgBotProxy"
                    :label="$t('setting.tgBotProxy')"
                    :hint="$t('setting.tgBotProxyHint')"
                    persistent-hint
                    placeholder="http://127.0.0.1:8080"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-combobox
                    v-model="tgBotOutbound"
                    :items="outboundChoices"
                    item-title="title"
                    item-value="value"
                    :label="$t('setting.tgBotOutbound')"
                    :hint="$t('setting.tgBotOutboundHint')"
                    persistent-hint
                    clearable
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-text-field
                    v-model="settings.tgBotReport"
                    :label="$t('setting.tgBotReport')"
                    :hint="$t('setting.tgBotReportHint')"
                    persistent-hint
                    placeholder="@daily"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-switch
                    v-model="tgBotReportBackup"
                    color="primary"
                    :label="$t('setting.tgBotReportBackup')"
                    hide-details
                  />
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="shop">
              <v-row>
                <v-col cols="12">
                  <v-alert
                    type="info"
                    variant="tonal"
                    density="compact"
                  >
                    {{ $t('setting.tgBotShopHint') }}
                  </v-alert>
                </v-col>
                <v-col cols="12">
                  <div class="text-subtitle-1 mb-2">
                    {{ $t('setting.tgBotShop') }}
                  </div>
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-textarea
                    :model-value="shop.welcomeFa"
                    :label="$t('setting.tgBotShopWelcome')"
                    rows="2"
                    auto-grow
                    hide-details
                    @update:model-value="(v: string) => patchShop({ welcomeFa: v })"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="6"
                >
                  <v-textarea
                    :model-value="shop.guideFa"
                    :label="$t('setting.tgBotShopGuide')"
                    rows="2"
                    auto-grow
                    hide-details
                    @update:model-value="(v: string) => patchShop({ guideFa: v })"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    :model-value="shop.payCard"
                    :label="$t('setting.tgBotShopPayCard')"
                    hide-details
                    @update:model-value="(v: string) => patchShop({ payCard: v })"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                  md="4"
                >
                  <v-text-field
                    :model-value="shop.payWallet"
                    :label="$t('setting.tgBotShopPayWallet')"
                    hide-details
                    @update:model-value="(v: string) => patchShop({ payWallet: v })"
                  />
                </v-col>
                <v-col
                  cols="12"
                  md="4"
                >
                  <v-text-field
                    :model-value="shop.payHint"
                    :label="$t('setting.tgBotShopPayHint')"
                    hide-details
                    @update:model-value="(v: string) => patchShop({ payHint: v })"
                  />
                </v-col>
                <v-col
                  cols="6"
                  md="3"
                >
                  <v-text-field
                    :model-value="String(shop.trialGB)"
                    type="number"
                    min="0"
                    :label="$t('setting.tgBotShopTrialGB')"
                    hide-details
                    @update:model-value="(v: string) => patchShop({ trialGB: Number(v) || 0 })"
                  />
                </v-col>
                <v-col
                  cols="6"
                  md="3"
                >
                  <v-text-field
                    :model-value="String(shop.trialHours)"
                    type="number"
                    min="0"
                    :label="$t('setting.tgBotShopTrialHours')"
                    hide-details
                    @update:model-value="(v: string) => patchShop({ trialHours: Number(v) || 0 })"
                  />
                </v-col>
                <v-col cols="12">
                  <div class="text-subtitle-2 mb-1">
                    {{ $t('setting.tgBotShopRefs') }}
                  </div>
                  <div class="text-caption text-medium-emphasis mb-2">
                    {{ $t('setting.tgBotShopRefHint') }}
                  </div>
                  <div
                    v-for="(r, i) in shop.refs"
                    :key="r.id"
                    class="d-flex flex-wrap ga-2 mb-2"
                  >
                    <v-text-field
                      :model-value="r.word"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopRefWord')"
                      style="min-width: 8rem"
                      @update:model-value="(v: string) => patchShopRef(i, { word: v })"
                    />
                    <v-text-field
                      :model-value="r.group"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopRefGroup')"
                      style="min-width: 8rem"
                      @update:model-value="(v: string) => patchShopRef(i, { group: v })"
                    />
                    <v-text-field
                      :model-value="r.admin"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopRefAdmin')"
                      style="min-width: 8rem"
                      @update:model-value="(v: string) => patchShopRef(i, { admin: v })"
                    />
                    <v-btn
                      icon="mdi-close"
                      variant="text"
                      size="small"
                      @click="removeShopRef(i)"
                    />
                  </div>
                  <v-btn
                    variant="tonal"
                    size="small"
                    @click="addShopRef"
                  >
                    {{ $t('setting.tgBotShopAddRef') }}
                  </v-btn>
                </v-col>
                <v-col cols="12">
                  <div class="text-subtitle-2 mb-2">
                    {{ $t('setting.tgBotShopPlans') }}
                  </div>
                  <div
                    v-for="(p, i) in shop.plans"
                    :key="p.id"
                    class="d-flex flex-wrap ga-2 mb-2"
                  >
                    <v-text-field
                      :model-value="p.name"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanName')"
                      style="min-width: 8rem"
                      @update:model-value="(v: string) => patchShopPlan(i, { name: v })"
                    />
                    <v-text-field
                      :model-value="String(p.days)"
                      type="number"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanDays')"
                      style="max-width: 6rem"
                      @update:model-value="(v: string) => patchShopPlan(i, { days: Number(v) || 0 })"
                    />
                    <v-text-field
                      :model-value="String(p.gb)"
                      type="number"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanGB')"
                      style="max-width: 8rem"
                      @update:model-value="(v: string) => patchShopPlan(i, { gb: Number(v) || 0 })"
                    />
                    <v-text-field
                      :model-value="p.price"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopPlanPrice')"
                      style="max-width: 8rem"
                      @update:model-value="(v: string) => patchShopPlan(i, { price: v })"
                    />
                    <v-btn
                      icon="mdi-close"
                      variant="text"
                      size="small"
                      @click="removeShopPlan(i)"
                    />
                  </div>
                  <v-btn
                    variant="tonal"
                    size="small"
                    @click="addShopPlan"
                  >
                    {{ $t('setting.tgBotShopAddPlan') }}
                  </v-btn>
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="desks">
              <v-row>
                <v-col cols="12">
                  <div class="text-subtitle-2 mb-1">
                    {{ $t('setting.tgBotShopDesks') }}
                  </div>
                  <div class="text-caption text-medium-emphasis mb-2">
                    {{ $t('setting.tgBotShopDesksHint') }}
                  </div>
                  <v-expansion-panels
                    variant="accordion"
                    class="mb-3"
                  >
                    <v-expansion-panel
                      v-for="id in shopAdminIds"
                      :key="id"
                    >
                      <v-expansion-panel-title>
                        {{ $t('setting.tgBotShopDeskTitle', { id }) }}
                      </v-expansion-panel-title>
                      <v-expansion-panel-text>
                        <div class="d-flex flex-wrap ga-2 mb-2">
                          <v-text-field
                            :model-value="deskOf(id).payCard"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPayCard')"
                            style="min-width: 10rem"
                            @update:model-value="(v: string) => patchShopDesk(id, { payCard: v })"
                          />
                          <v-text-field
                            :model-value="deskOf(id).payWallet"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPayWallet')"
                            style="min-width: 10rem"
                            @update:model-value="(v: string) => patchShopDesk(id, { payWallet: v })"
                          />
                          <v-text-field
                            :model-value="deskOf(id).payHint"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPayHint')"
                            style="min-width: 10rem"
                            @update:model-value="(v: string) => patchShopDesk(id, { payHint: v })"
                          />
                        </div>
                        <div
                          v-for="(p, i) in deskOf(id).plans"
                          :key="p.id"
                          class="d-flex flex-wrap ga-2 mb-2"
                        >
                          <v-text-field
                            :model-value="p.name"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPlanName')"
                            style="min-width: 8rem"
                            @update:model-value="(v: string) => patchShopDeskPlan(id, i, { name: v })"
                          />
                          <v-text-field
                            :model-value="String(p.days)"
                            type="number"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPlanDays')"
                            style="max-width: 6rem"
                            @update:model-value="(v: string) => patchShopDeskPlan(id, i, { days: Number(v) || 0 })"
                          />
                          <v-text-field
                            :model-value="String(p.gb)"
                            type="number"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPlanGB')"
                            style="max-width: 8rem"
                            @update:model-value="(v: string) => patchShopDeskPlan(id, i, { gb: Number(v) || 0 })"
                          />
                          <v-text-field
                            :model-value="p.price"
                            density="compact"
                            hide-details
                            :label="$t('setting.tgBotShopPlanPrice')"
                            style="max-width: 8rem"
                            @update:model-value="(v: string) => patchShopDeskPlan(id, i, { price: v })"
                          />
                          <v-btn
                            icon="mdi-close"
                            variant="text"
                            size="small"
                            @click="removeShopDeskPlan(id, i)"
                          />
                        </div>
                        <v-btn
                          variant="tonal"
                          size="small"
                          class="me-2"
                          @click="addShopDeskPlan(id)"
                        >
                          {{ $t('setting.tgBotShopAddPlan') }}
                        </v-btn>
                        <v-btn
                          variant="text"
                          size="small"
                          color="error"
                          @click="removeShopDesk(id)"
                        >
                          {{ $t('setting.tgBotShopRemoveDesk') }}
                        </v-btn>
                      </v-expansion-panel-text>
                    </v-expansion-panel>
                  </v-expansion-panels>
                  <div class="d-flex flex-wrap ga-2 align-center">
                    <v-text-field
                      v-model="newDeskId"
                      density="compact"
                      hide-details
                      :label="$t('setting.tgBotShopDeskId')"
                      style="max-width: 16rem"
                    />
                    <v-btn
                      variant="tonal"
                      size="small"
                      @click="addShopDesk"
                    >
                      {{ $t('setting.tgBotShopAddDesk') }}
                    </v-btn>
                  </div>
                </v-col>
              </v-row>
            </v-window-item>
          </v-window>
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
import SubJsonExtVue from '@/components/SubJsonExt.vue'
import SubClashExtVue from '@/components/SubClashExt.vue'
import PanelUpdateVue from '@/components/PanelUpdate.vue'
import { push } from 'notivue'
import Data from '@/store/modules/data'
// The Update tab reloads the page into the new version once an update is
// done, and marks that it wants to be open again.
const tab = ref(sessionStorage.getItem('panelUpdate.reopen') ? 't7' : 't1')
sessionStorage.removeItem('panelUpdate.reopen')
const tgTab = ref('bot')
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
  const msg = await HttpUtils.post<{ settings: PanelSettings }>('api/save', { object: 'settings', action: 'set', data: JSON.stringify(settings.value) })
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

const tgBotEnable = computed({
  get: () => { return settings.value.tgBotEnable == "true" },
  set: (v:boolean) => { settings.value.tgBotEnable = v ? "true" : "false" }
})
const tgBotReportBackup = computed({
  get: () => { return settings.value.tgBotReportBackup == "true" },
  set: (v:boolean) => { settings.value.tgBotReportBackup = v ? "true" : "false" }
})
const tgBotPanel = computed({
  get: () => { return settings.value.tgBotPanel == "true" },
  set: (v:boolean) => { settings.value.tgBotPanel = v ? "true" : "false" }
})
const tgBotNotify = computed({
  get: () => { return settings.value.tgBotNotify == "true" },
  set: (v:boolean) => { settings.value.tgBotNotify = v ? "true" : "false" }
})

type OutboundChoice = { title: string, value: string }
const outboundChoices = computed((): OutboundChoice[] => {
  const none: OutboundChoice = { title: i18n.global.t('setting.tgBotOutboundNone'), value: '' }
  const seen = new Set<string>()
  const items: OutboundChoice[] = [none]
  for (const o of (Data().outbounds ?? [])) {
    const tag = String((o as { tag?: string }).tag || '').trim()
    if (!tag || seen.has(tag)) continue
    seen.add(tag)
    const typ = String((o as { type?: string }).type || '')
    items.push({ title: typ ? `${tag} (${typ})` : tag, value: tag })
  }
  const cur = String(settings.value.tgBotOutbound || '').trim()
  if (cur && !seen.has(cur)) items.push({ title: cur, value: cur })
  return items
})
const tgBotOutbound = computed({
  get: () => settings.value.tgBotOutbound || '',
  set: (v: unknown) => {
    if (v && typeof v === 'object' && v !== null && 'value' in v) {
      settings.value.tgBotOutbound = String((v as { value: string }).value || '')
      return
    }
    settings.value.tgBotOutbound = String(v || '')
  }
})

type ShopPlan = { id: string, name: string, days: number, gb: number, price: string }
type ShopRef = { id: string, word: string, group: string, admin: string }
type ShopDesk = { payCard: string, payWallet: string, payHint: string, plans: ShopPlan[] }
type Shop = {
  welcomeFa: string
  guideFa: string
  payCard: string
  payWallet: string
  payHint: string
  trialGB: number
  trialHours: number
  plans: ShopPlan[]
  refs: ShopRef[]
  desks: Record<string, ShopDesk>
}

const emptyDesk = (): ShopDesk => ({ payCard: '', payWallet: '', payHint: '', plans: [] })
const emptyShop = (): Shop => ({
  welcomeFa: '', guideFa: '', payCard: '', payWallet: '', payHint: '',
  trialGB: 0, trialHours: 0, plans: [], refs: [], desks: {},
})

const parsePlans = (plans: unknown): ShopPlan[] => {
  if (!Array.isArray(plans)) return []
  return plans.map((p: { id?: string, name?: string, days?: number, gb?: number, price?: string }, i: number) => ({
    id: String(p?.id || String(i + 1)),
    name: String(p?.name || ''),
    days: Math.max(0, Math.trunc(Number(p?.days) || 0)),
    gb: Math.max(0, Math.trunc(Number(p?.gb) || 0)),
    price: String(p?.price || ''),
  }))
}

const parseDesk = (o: { payCard?: string, payWallet?: string, payHint?: string, plans?: unknown } | undefined): ShopDesk => ({
  payCard: String(o?.payCard || ''),
  payWallet: String(o?.payWallet || ''),
  payHint: String(o?.payHint || ''),
  plans: parsePlans(o?.plans),
})

const parseShop = (raw: string): Shop => {
  try {
    const o = JSON.parse(raw || '{}')
    const desksIn = o.desks && typeof o.desks === 'object' ? o.desks as Record<string, { payCard?: string, payWallet?: string, payHint?: string, plans?: unknown }> : {}
    const desks: Record<string, ShopDesk> = {}
    for (const [id, d] of Object.entries(desksIn)) {
      if (/^\d+$/.test(id)) desks[id] = parseDesk(d)
    }
    const refs = Array.isArray(o.refs) ? o.refs : []
    return {
      welcomeFa: String(o.welcomeFa || ''),
      guideFa: String(o.guideFa || ''),
      payCard: String(o.payCard || ''),
      payWallet: String(o.payWallet || ''),
      payHint: String(o.payHint || ''),
      trialGB: Math.max(0, Math.trunc(Number(o.trialGB) || 0)),
      trialHours: Math.max(0, Math.trunc(Number(o.trialHours) || 0)),
      plans: parsePlans(o.plans),
      refs: refs.map((r: { id?: string, word?: string, group?: string, admin?: number | string }, i: number) => ({
        id: String(r?.id || String(i + 1)),
        word: String(r?.word || ''),
        group: String(r?.group || ''),
        admin: r?.admin ? String(r.admin) : '',
      })),
      desks,
    }
  } catch {
    return emptyShop()
  }
}

const writeShop = (s: Shop) => {
  let cur: Record<string, unknown>
  try { cur = JSON.parse(settings.value.tgBotShop || '{}') || {} } catch { cur = {} }
  const refs = s.refs.map((r) => {
    const admin = Number(r.admin)
    return admin > 0 ? { id: r.id, word: r.word, group: r.group, admin } : { id: r.id, word: r.word, group: r.group }
  })
  settings.value.tgBotShop = JSON.stringify({
    ...cur,
    welcomeFa: s.welcomeFa, guideFa: s.guideFa,
    payCard: s.payCard, payWallet: s.payWallet, payHint: s.payHint,
    trialGB: s.trialGB, trialHours: s.trialHours,
    plans: s.plans,
    refs,
    desks: s.desks,
  })
}

const shop = computed(() => parseShop(settings.value.tgBotShop || '{}'))

const patchShop = (partial: Partial<Shop>) => {
  writeShop({ ...shop.value, ...partial })
}

const patchShopPlan = (i: number, partial: Partial<ShopPlan>) => {
  const plans = shop.value.plans.map((p, idx) => idx === i ? { ...p, ...partial } : p)
  writeShop({ ...shop.value, plans })
}

const addShopPlan = () => {
  const s = shop.value
  const id = String((s.plans.reduce((m, p) => Math.max(m, Number(p.id) || 0), 0) || 0) + 1)
  writeShop({ ...s, plans: [...s.plans, { id, name: '', days: 30, gb: 50, price: '' }] })
}

const removeShopPlan = (i: number) => {
  writeShop({ ...shop.value, plans: shop.value.plans.filter((_, idx) => idx !== i) })
}

const patchShopRef = (i: number, partial: Partial<ShopRef>) => {
  const refs = shop.value.refs.map((r, idx) => idx === i ? { ...r, ...partial } : r)
  writeShop({ ...shop.value, refs })
}

const addShopRef = () => {
  const s = shop.value
  const id = String((s.refs.reduce((m, r) => Math.max(m, Number(r.id) || 0), 0) || 0) + 1)
  writeShop({ ...s, refs: [...s.refs, { id, word: '', group: '', admin: '' }] })
}

const removeShopRef = (i: number) => {
  writeShop({ ...shop.value, refs: shop.value.refs.filter((_, idx) => idx !== i) })
}

const newDeskId = ref('')
const shopAdminIds = computed(() => {
  const ids: string[] = []
  const add = (s: string) => {
    const v = String(s || '').trim()
    if (/^\d+$/.test(v) && !ids.includes(v)) ids.push(v)
  }
  add(settings.value.tgBotOwner)
  String(settings.value.tgBotAdmins || '').split(/[,\s]+/).forEach(add)
  Object.keys(shop.value.desks).forEach(add)
  return ids
})
const deskOf = (id: string): ShopDesk => shop.value.desks[id] || emptyDesk()
const patchShopDesk = (id: string, partial: Partial<ShopDesk>) => {
  const desks = { ...shop.value.desks, [id]: { ...deskOf(id), ...partial } }
  writeShop({ ...shop.value, desks })
}
const patchShopDeskPlan = (id: string, i: number, partial: Partial<ShopPlan>) => {
  const d = deskOf(id)
  const plans = d.plans.map((p, idx) => idx === i ? { ...p, ...partial } : p)
  patchShopDesk(id, { plans })
}
const addShopDeskPlan = (id: string) => {
  const d = deskOf(id)
  const pid = String((d.plans.reduce((m, p) => Math.max(m, Number(p.id) || 0), 0) || 0) + 1)
  patchShopDesk(id, { plans: [...d.plans, { id: pid, name: '', days: 30, gb: 50, price: '' }] })
}
const removeShopDeskPlan = (id: string, i: number) => {
  patchShopDesk(id, { plans: deskOf(id).plans.filter((_, idx) => idx !== i) })
}
const addShopDesk = () => {
  const id = newDeskId.value.trim()
  if (!/^\d+$/.test(id)) return
  if (!shop.value.desks[id]) patchShopDesk(id, emptyDesk())
  const listed = String(settings.value.tgBotAdmins || '')
  if (!listed.split(/[,\s]+/).includes(id) && settings.value.tgBotOwner.trim() !== id) {
    settings.value.tgBotAdmins = listed.trim() ? listed.trim().replace(/,?\s*$/, '') + ', ' + id : id
  }
  newDeskId.value = ''
}
const removeShopDesk = (id: string) => {
  const desks = { ...shop.value.desks }
  delete desks[id]
  writeShop({ ...shop.value, desks })
}

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
