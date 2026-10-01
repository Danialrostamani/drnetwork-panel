<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="520"
  >
    <v-card
      class="rounded-lg"
      :loading="loading"
    >
      <v-card-title>{{ $t('client.onlineIps') }} · {{ name }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-alert
          v-if="!loading && problems.length > 0"
          type="warning"
          variant="tonal"
          class="mb-3"
        >
          <div
            v-for="problem in problems"
            :key="problem"
          >
            {{ problem }}
          </div>
        </v-alert>
        <v-alert
          v-if="!loading && ips.length === 0"
          type="info"
          variant="tonal"
        >
          {{ $t('noData') }}
        </v-alert>
        <v-list
          v-else
          density="compact"
        >
          <v-list-item
            v-for="item in ips"
            :key="item.ip"
            :title="item.ip"
            :subtitle="item.since ? new Date(item.since * 1000).toLocaleString(locale) : undefined"
          >
            <template #prepend>
              <v-icon
                :color="item.idle ? 'warning' : 'success'"
                icon="mdi-ip-network"
              />
            </template>
            <template #append>
              <v-chip
                v-if="item.idle"
                color="warning"
                size="small"
              >
                {{ $t('client.idle') }}
              </v-chip>
            </template>
          </v-list-item>
        </v-list>
      </v-card-text>
      <v-card-actions>
        <v-spacer /><v-btn
          color="primary"
          variant="outlined"
          @click="$emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { locale } from '@/locales'
interface OnlineIP { ip: string; since?: number; idle?: boolean | null }
const props = defineProps<{ visible: boolean; name: string }>()
defineEmits<{ close: [] }>()
const loading = ref(false)
const ips = ref<OnlineIP[]>([])
const problems = ref<string[]>([])
watch(() => props.visible, async v => {
  if (!v) return
  loading.value = true
  const msg = await HttpUtils.get<{ ips: OnlineIP[]; problems?: string[] }>('api/onlineIps', { name: props.name })
  ips.value = msg.success ? (msg.obj?.ips ?? []) : []
  problems.value = msg.success ? (msg.obj?.problems ?? []) : []
  loading.value = false
})
</script>
