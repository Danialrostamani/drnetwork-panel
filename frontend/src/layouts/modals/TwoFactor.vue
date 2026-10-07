<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="420"
  >
    <v-card
      class="rounded-lg"
      :loading="busy"
    >
      <v-card-title>
        <v-row>
          <v-col>{{ $t('admin.totp.title') }}</v-col>
          <v-spacer />
          <v-col cols="auto">
            <v-icon
              icon="mdi-close-box"
              @click="$emit('close')"
            />
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <template v-if="state.enabled">
          <v-alert
            type="success"
            density="compact"
            class="mb-4"
          >
            {{ $t('admin.totp.on') }}
          </v-alert>
          <p class="mb-2">
            {{ $t('admin.totp.offHint') }}
          </p>
        </template>
        <template v-else-if="state.uri">
          <p class="mb-2">
            {{ $t('admin.totp.scan') }}
          </p>
          <div style="text-align: center;">
            <QrcodeVue
              :value="state.uri"
              :size="200"
              :margin="1"
              style="border-radius: 1rem;"
            />
          </div>
          <v-text-field
            :model-value="state.secret"
            :label="$t('admin.totp.secret')"
            readonly
            density="compact"
            class="mt-2"
            append-inner-icon="mdi-content-copy"
            @click:append-inner="copy(state.secret)"
          />
        </template>
        <v-text-field
          v-model="code"
          :label="$t('login.code')"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          v-if="state.enabled"
          color="error"
          variant="tonal"
          :disabled="code.length != 6"
          :loading="busy"
          @click="save('disable')"
        >
          {{ $t('admin.totp.disable') }}
        </v-btn>
        <v-btn
          v-else
          color="primary"
          variant="tonal"
          :disabled="code.length != 6 || !state.uri"
          :loading="busy"
          @click="save('enable')"
        >
          {{ $t('admin.totp.enable') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import QrcodeVue from 'qrcode.vue'
import HttpUtils from '@/plugins/httputil'
import { ref, watch } from 'vue'

interface TotpState {
  enabled: boolean
  secret: string
  uri: string
}

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const state = ref<TotpState>({ enabled: false, secret: '', uri: '' })
const code = ref('')
const busy = ref(false)

const load = async () => {
  busy.value = true
  const msg = await HttpUtils.get<TotpState>('api/totp')
  busy.value = false
  if (msg.success && msg.obj) state.value = msg.obj
}

watch(() => props.visible, (v) => {
  code.value = ''
  if (v) load()
})

const save = async (action: string) => {
  busy.value = true
  const msg = await HttpUtils.post('api/totp', { action: action, code: code.value })
  busy.value = false
  if (msg.success) emit('close')
}

const copy = (text: string) => {
  navigator.clipboard?.writeText(text)
}
</script>
