<template>
  <v-card
    variant="outlined"
    class="bot-access mb-6"
    rounded="lg"
  >
    <v-card-text>
      <div class="d-flex align-center ga-3 mb-3">
        <v-icon
          icon="mdi-shield-crown-outline"
          color="primary"
          size="30"
        />
        <div>
          <h2 class="text-h6">
            {{ $t('botAccess.title') }}
          </h2><p class="text-body-2 text-medium-emphasis">
            {{ $t('botAccess.subtitle') }}
          </p>
        </div>
      </div>
      <v-row>
        <v-col
          cols="12"
          md="6"
        >
          <v-text-field
            v-model="settings.tgBotOwner"
            :label="$t('setting.tgBotOwner')"
            :hint="$t('setting.tgBotOwnerHint')"
            persistent-hint
            inputmode="numeric"
            autocomplete="off"
            prepend-inner-icon="mdi-crown-outline"
            :rules="[v => !v || !!telegramId(String(v)) || $t('botAccess.invalidId')]"
            @blur="normalizeOwner"
          />
        </v-col>
        <v-col
          cols="12"
          md="6"
        >
          <v-switch
            v-model="privateMode"
            color="primary"
            :label="$t('botAccess.private')"
            :hint="$t('botAccess.privateHint')"
            persistent-hint
          />
        </v-col>
      </v-row>
      <v-divider class="my-5" />
      <h3 class="text-subtitle-1 font-weight-bold mb-2">
        {{ $t('botAccess.partner') }}
      </h3>
      <p class="text-body-2 text-medium-emphasis mb-4">
        {{ $t('botAccess.partnerHint') }}
      </p>
      <v-alert
        v-if="!owner || settings.tgBotOwner?.trim() !== String(owner)"
        type="warning"
        variant="tonal"
        class="mb-4"
      >
        {{ $t('botAccess.saveOwner') }}
      </v-alert>
      <v-form @submit.prevent="grant">
        <v-row align="start">
          <v-col
            cols="12"
            md="8"
          >
            <v-text-field
              v-model="partnerId"
              :label="$t('botAccess.partnerId')"
              :hint="$t('botAccess.idHint')"
              persistent-hint
              inputmode="numeric"
              autocomplete="off"
              placeholder="123456789"
              :error-messages="partnerId && !validId ? [$t('botAccess.invalidId')] : []"
            />
          </v-col>
          <v-col
            cols="12"
            md="4"
          >
            <v-btn
              type="submit"
              color="primary"
              variant="flat"
              size="large"
              block
              :loading="busy"
              :disabled="!validId || !owner || settings.tgBotOwner?.trim() !== String(owner) || validId === String(owner)"
              prepend-icon="mdi-account-key-outline"
            >
              {{ $t('botAccess.grant') }}
            </v-btn>
          </v-col>
        </v-row>
      </v-form>
      <p class="text-body-2 mt-4 mb-2">
        {{ $t('botAccess.current') }}
      </p>
      <div
        class="d-flex flex-wrap ga-2"
        aria-live="polite"
      >
        <v-chip
          v-if="owner"
          color="primary"
          variant="tonal"
          prepend-icon="mdi-crown"
        >
          {{ $t('botAccess.owner') }} · <bdi>{{ owner }}</bdi>
        </v-chip>
        <v-chip
          v-for="a in partners"
          :key="a.id"
          :color="a.suspended ? 'warning' : 'success'"
          variant="tonal"
          :prepend-icon="a.suspended ? 'mdi-pause-circle-outline' : 'mdi-shield-account-outline'"
        >
          {{ a.name || a.id }}{{ a.suspended ? ' · ' + $t('botAccess.suspended') : '' }}
        </v-chip>
      </div>
      <v-alert
        type="info"
        variant="tonal"
        class="mt-4"
        density="compact"
      >
        {{ $t('botAccess.trust') }}
      </v-alert>
      <v-btn
        class="mt-3"
        variant="text"
        prepend-icon="mdi-account-cog-outline"
        :to="{ query: { tab: 'sellers' } }"
      >
        {{ $t('botAccess.manage') }}
      </v-btn>
    </v-card-text>
  </v-card>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { push } from 'notivue'
import HttpUtils from '@/plugins/httputil'
import { useBotSettings } from '@/plugins/botSettings'
import { telegramId } from '@/plugins/telegramId'
import { i18n } from '@/locales'
const { settings, botChanged, refreshAccess } = useBotSettings()
const normalizeOwner = () => { const id = telegramId(settings.value.tgBotOwner ?? ''); if (id) settings.value.tgBotOwner = id }
const owner = ref(0)
const partners = ref<{ id: number, name: string, suspended: boolean }[]>([])
const partnerId = ref('')
const validId = computed(() => telegramId(partnerId.value))
const busy = ref(false)
const privateMode = computed({ get: () => settings.value.tgBotPrivate !== 'false', set: v => { settings.value.tgBotPrivate = String(v) } })
const load = async () => {
  const msg = await HttpUtils.get<{ owner: number, admins: { id: number, name: string, role: string, suspended: boolean }[] }>('api/botAdmins')
  if (msg.success && msg.obj) { owner.value = msg.obj.owner; partners.value = msg.obj.admins.filter(a => a.role === 'full') }
}
const grant = async () => {
  if (busy.value || !validId.value || !owner.value || settings.value.tgBotOwner?.trim() !== String(owner.value) || validId.value === String(owner.value)) return
  if (!confirm(i18n.global.t('botAccess.confirm', { id: validId.value }))) return
  busy.value = true
  try {
    const msg = await HttpUtils.post('api/botPartner', { id: validId.value })
    if (msg.success) { partnerId.value = ''; await refreshAccess(); await load(); push.success({ message: i18n.global.t('botAccess.granted') }) }
  } finally { busy.value = false }
}
onMounted(load)
watch(botChanged, changed => { if (!changed) void load() })
</script>

<style scoped>
.bot-access { border-color: rgba(var(--v-theme-on-surface), .18); }
.bot-access h2, .bot-access h3, .bot-access p { margin-top: 0; }
:deep(.v-messages) { font-size: .875rem; }
:deep(.v-messages__message) { line-height: 1.5; }
</style>
