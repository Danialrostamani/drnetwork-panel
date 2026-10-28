<template>
  <div class="bot-heading mb-5 d-flex align-start flex-wrap ga-4">
    <v-avatar
      color="primary"
      variant="tonal"
      rounded="lg"
      size="52"
    >
      <v-icon
        icon="mdi-robot-outline"
        size="30"
      />
    </v-avatar>
    <div class="flex-grow-1">
      <h1 class="text-h5 font-weight-bold">
        {{ $t('botAccess.pageTitle') }}
      </h1><p class="text-body-2 text-medium-emphasis mt-1">
        {{ $t('botAccess.pageHint') }}
      </p>
    </div>
    <v-btn
      variant="tonal"
      color="primary"
      prepend-icon="mdi-shield-account-outline"
      @click="tab = 'settings'"
    >
      {{ $t('botAccess.accessSettings') }}
    </v-btn>
  </div>
  <v-card
    rounded="lg"
    :loading="saving"
  >
    <v-tabs
      v-model="tab"
      color="primary"
      align-tabs="start"
      show-arrows
    >
      <v-tab
        v-for="tb in tabs"
        :key="tb.value"
        :value="tb.value"
        :prepend-icon="tb.icon"
      >
        {{ $t(tb.title) }}
      </v-tab>
    </v-tabs>
    <v-card-text>
      <v-alert
        v-if="botChanged"
        type="warning"
        variant="tonal"
        density="compact"
        class="mb-3"
        style="position: sticky; top: 64px; z-index: 5;"
      >
        {{ $t('botPage.unsaved') }}
        <template #append>
          <v-btn
            variant="text"
            size="small"
            :disabled="saving"
            @click="discardBotSettings"
          >
            {{ $t('botPage.discard') }}
          </v-btn>
          <v-btn
            color="primary"
            variant="flat"
            size="small"
            :loading="saving"
            @click="save"
          >
            {{ $t('actions.save') }}
          </v-btn>
        </template>
      </v-alert>
      <v-window v-model="tab">
        <v-window-item value="dashboard">
          <BotDashboard
            ref="dashRef"
            @go="(v: string) => tab = v"
          />
        </v-window-item>
        <v-window-item value="orders">
          <BotOrders @changed="dashRef?.load()" />
        </v-window-item>
        <v-window-item value="sellers">
          <BotSellers v-if="loaded" />
        </v-window-item>
        <v-window-item value="customers">
          <BotCustomers />
        </v-window-item>
        <v-window-item value="shop">
          <BotShop v-if="loaded" />
        </v-window-item>
        <v-window-item value="settings">
          <BotConfig v-if="loaded" />
        </v-window-item>
      </v-window>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { i18n } from '@/locales'
import { push } from 'notivue'
import { onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { useBotSettings } from '@/plugins/botSettings'
import BotDashboard from '@/components/bot/BotDashboard.vue'
import BotSellers from '@/components/bot/BotSellers.vue'
import BotOrders from '@/components/bot/BotOrders.vue'
import BotCustomers from '@/components/bot/BotCustomers.vue'
import BotShop from '@/components/bot/BotShop.vue'
import BotConfig from '@/components/bot/BotConfig.vue'

// Everything about the Telegram bot, in one page: what used to be split
// between the Settings page and the bot members page.

const tabs = [
  { value: 'dashboard', icon: 'mdi-view-dashboard', title: 'botPage.dashboard' },
  { value: 'orders', icon: 'mdi-receipt-text', title: 'botPage.orders' },
  { value: 'sellers', icon: 'mdi-account-tie', title: 'botPage.sellers' },
  { value: 'customers', icon: 'mdi-account-multiple', title: 'botPage.customers' },
  { value: 'shop', icon: 'mdi-store', title: 'botPage.shop' },
  { value: 'settings', icon: 'mdi-cog', title: 'botPage.settings' },
]

const route = useRoute()
const router = useRouter()
const known = (v: unknown) => tabs.some(x => x.value == v)
const tab = ref<string>(known(route.query.tab) ? String(route.query.tab) : 'dashboard')
watch(tab, (v) => { router.replace({ query: { ...route.query, tab: v } }) })
watch(() => route.query.tab, v => { if (known(v)) tab.value = String(v) })

const { loaded, botChanged, loadBotSettings, saveBotSettings, discardBotSettings } = useBotSettings()
const saving = ref(false)
const dashRef = ref<{ load: () => Promise<void> } | null>(null)

onMounted(loadBotSettings)

const save = async () => {
  saving.value = true
  const ok = await saveBotSettings()
  saving.value = false
  if (ok) {
    push.success({ title: i18n.global.t('success'), duration: 4000, message: i18n.global.t('botPage.saved') })
    dashRef.value?.load()
  }
}

onBeforeRouteLeave(() => {
  if (!botChanged.value) return true
  if (!confirm(i18n.global.t('botPage.leaveConfirm'))) return false
  discardBotSettings()
  return true
})
</script>

<style scoped>
.bot-heading {
  display: grid !important;
  grid-template-columns: 52px minmax(0, 1fr) auto;
  align-items: center !important;
  gap: 16px;
}
.bot-heading h1, .bot-heading p { margin: 0 !important; }
.bot-heading p { margin-top: 8px !important; }
@media (max-width: 600px) {
  .bot-heading { grid-template-columns: 44px minmax(0, 1fr); }
  .bot-heading .v-btn { grid-column: 1 / -1; justify-self: start; }
  .bot-heading h1 { font-size: 1.25rem !important; }
}
</style>
