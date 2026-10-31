<template>
  <BotPartners />
  <h2 class="text-h6 mb-1">
    {{ $t('botAccess.connection') }}
  </h2>
  <p class="text-body-2 text-medium-emphasis mb-4">
    {{ $t('botAccess.connectionHint') }}
  </p>
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
        v-if="settings.tgBotPrivate === 'false'"
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
        autocomplete="new-password"
        :type="showToken ? 'text' : 'password'"
        hide-details
      >
        <template #append-inner>
          <v-btn
            :icon="showToken ? 'mdi-eye-off' : 'mdi-eye'"
            :aria-label="$t(showToken ? 'botAccess.hideToken' : 'botAccess.showToken')"
            variant="text"
            size="small"
            @click="showToken = !showToken"
          />
        </template>
      </v-text-field>
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
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import BotPartners from './BotPartners.vue'
import Data from '@/store/modules/data'
import { useBotSettings } from '@/plugins/botSettings'

// The bot's own settings: token, owner, language, how it reaches Telegram.
// Who its administrators are is on the sellers tab.
const { settings, outboundChoices, tgBotOutbound, tgBotEnable, tgBotNotify, tgBotPanel, tgBotReportBackup } = useBotSettings()

const showToken = ref(false)

onMounted(() => { Data().loadData() })
</script>

<style scoped>
:deep(.v-messages) { font-size: .875rem; }
:deep(.v-messages__message) { line-height: 1.5; }
</style>
