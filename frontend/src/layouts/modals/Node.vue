<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="650"
  >
    <v-card
      class="rounded-lg"
      :loading="loading"
    >
      <v-card-title>{{ isNew ? $t('actions.add') : $t('actions.edit') }} {{ $t('objects.node') }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-container>
          <v-row>
            <v-col
              cols="12"
              sm="6"
            >
              <v-text-field
                v-model.trim="node.name"
                :label="$t('node.name')"
                placeholder="tokyo-1"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
            >
              <v-switch
                v-model="node.enable"
                color="primary"
                :label="$t('enable')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12">
              <v-text-field
                v-model.trim="node.baseUrl"
                dir="ltr"
                :label="$t('node.baseUrl')"
                :hint="$t('node.baseUrlHint')"
                persistent-hint
                placeholder="https://1.2.3.4:2095"
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="5"
            >
              <v-text-field
                v-model.trim="node.webPath"
                dir="ltr"
                :label="$t('node.webPath')"
                placeholder="/app/"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="7"
            >
              <v-text-field
                v-model="node.token"
                type="password"
                autocomplete="new-password"
                dir="ltr"
                :label="$t('node.token')"
                :placeholder="!isNew && node.tokenSet ? $t('node.tokenKeep') : ''"
                :hint="$t('node.tokenHint')"
                persistent-hint
              />
            </v-col>
          </v-row>
          <template v-if="isHttps">
            <v-row>
              <v-col cols="12">
                <v-switch
                  v-model="node.insecure"
                  color="warning"
                  :label="$t('node.insecure')"
                  hide-details
                />
              </v-col>
            </v-row>
            <v-row>
              <v-col cols="12">
                <v-text-field
                  v-model.trim="node.certPin"
                  dir="ltr"
                  :label="$t('node.certPin')"
                  :hint="$t('node.certPinHint')"
                  persistent-hint
                />
              </v-col>
            </v-row>
          </template>
          <v-alert
            v-else-if="node.baseUrl.startsWith('http://')"
            type="warning"
            variant="tonal"
            density="compact"
            class="mt-4"
          >
            {{ $t('node.httpWarn') }}
          </v-alert>
          <v-row>
            <v-col cols="12">
              <v-text-field
                v-model="node.desc"
                :label="$t('node.desc')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row align="center">
            <v-col cols="auto">
              <v-btn
                variant="tonal"
                color="primary"
                :loading="testing"
                prepend-icon="mdi-connection"
                @click="testConnection"
              >
                {{ $t('node.test') }}
              </v-btn>
            </v-col>
            <v-col v-if="testResult">
              <v-chip
                :color="statusColor(testResult.state)"
                size="small"
                label
              >
                {{ statusLabel(testResult.state) }}
                <template v-if="testResult.state === 'online'">
                  · {{ testResult.latency }} ms · {{ testResult.appVersion }} / {{ testResult.coreVersion }}
                </template>
              </v-chip>
              <div
                v-if="testResult.error"
                class="text-error text-caption mt-1"
              >
                {{ testResult.error }}
              </div>
            </v-col>
          </v-row>
        </v-container>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="$emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          :disabled="!canSubmit"
          @click="save"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import { defaultNode, type Node, type NodeStatus } from '@/types/node'
import { i18n } from '@/locales'

const props = defineProps<{ visible: boolean; item?: Node | null }>()
const emit = defineEmits<{ close: [] }>()
const node = ref<Node>({ ...defaultNode })
const loading = ref(false)
const testing = ref(false)
const testResult = ref<NodeStatus | null>(null)
const isNew = computed(() => !node.value.id)
const isHttps = computed(() => node.value.baseUrl.toLowerCase().startsWith('https://'))
const canSubmit = computed(() => !!node.value.name.trim() && !!node.value.baseUrl.trim() && (!isNew.value || !!node.value.token))

watch(() => props.visible, (visible) => {
  if (!visible) return
  node.value = props.item ? { ...defaultNode, ...JSON.parse(JSON.stringify(props.item)), token: '' } : { ...defaultNode }
  testResult.value = null
})

const statusColor = (state: string) => state === 'online' ? 'success' : state === 'core-stopped' ? 'warning' : 'error'
const statusLabel = (state: string) => i18n.global.t(`node.status.${state === 'core-stopped' ? 'coreStopped' : state}`)

const testConnection = async () => {
  testing.value = true
  testResult.value = null
  const msg = await HttpUtils.post<NodeStatus>('api/testNode', { data: JSON.stringify(node.value) })
  testing.value = false
  if (msg.success) testResult.value = msg.obj
}

const save = async () => {
  loading.value = true
  const success = await Data().save('nodes', isNew.value ? 'new' : 'edit', node.value)
  loading.value = false
  if (success) emit('close')
}
</script>
