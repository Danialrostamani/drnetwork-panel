<template>
  <v-row align="center">
    <v-col
      cols="6"
      sm="4"
      md="3"
    >
      <v-select
        v-model="level"
        density="compact"
        hide-details
        :label="$t('basic.log.level')"
        :items="levels"
        @update:model-value="load"
      />
    </v-col>
    <v-col
      cols="6"
      sm="4"
      md="3"
    >
      <v-select
        v-model.number="count"
        density="compact"
        hide-details
        :label="$t('count')"
        :items="[20, 50, 100, 200, 500]"
        @update:model-value="load"
      />
    </v-col>
    <v-col cols="auto">
      <v-btn
        icon="mdi-refresh"
        variant="tonal"
        size="small"
        :loading="loading"
        @click="load"
      />
    </v-col>
  </v-row>
  <!-- Text, not markup: log lines quote what clients sent. -->
  <v-card
    class="mt-2"
    color="background"
    dir="ltr"
  >
    <pre class="node-log">{{ lines.length > 0 ? lines.join('\n') : (loaded ? $t('noData') : '') }}</pre>
  </v-card>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'

const props = defineProps<{ nodeId: number }>()
const lines = ref<string[]>([])
const loading = ref(false)
const loaded = ref(false)
const level = ref('info')
const count = ref(50)
const levels = [
  { title: 'DEBUG', value: 'debug' },
  { title: 'INFO', value: 'info' },
  { title: 'WARNING', value: 'warning' },
  { title: 'ERROR', value: 'err' },
]

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<string[]>('api/nodeLogs', { id: props.nodeId, c: count.value, l: level.value })
  loading.value = false
  loaded.value = true
  lines.value = msg.success && Array.isArray(msg.obj) ? msg.obj.map(String) : []
}
onMounted(load)
</script>

<style scoped>
.node-log {
  margin: 0;
  padding: .5rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-family: monospace;
  font-size: .8rem;
  max-height: 55vh;
  overflow-y: auto;
}
</style>
