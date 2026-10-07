<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="760"
    scrollable
  >
    <v-card
      class="rounded-lg"
      :loading="saving"
    >
      <v-card-title>{{ $t('node.multi.title') }}</v-card-title>
      <v-divider />
      <v-card-text>
        <p class="mb-3">
          {{ $t('node.multi.hint') }}
        </p>
        <v-textarea
          v-model="text"
          dir="ltr"
          rows="8"
          auto-grow
          class="node-multi"
          :placeholder="placeholder"
          persistent-placeholder
          hide-details
        />
        <v-checkbox
          v-model="insecure"
          color="warning"
          :label="$t('node.multi.insecure')"
          hide-details
        />
        <div
          v-if="text.trim()"
          class="mt-2"
        >
          <v-chip
            :color="parsed.errors.length === 0 && parsed.nodes.length > 0 ? 'success' : 'default'"
            label
            size="small"
          >
            {{ $t('node.multi.ready', { n: parsed.nodes.length }) }}
          </v-chip>
          <v-alert
            v-if="parsed.errors.length > 0"
            type="error"
            variant="tonal"
            density="compact"
            class="mt-2"
          >
            <div
              v-for="e in parsed.errors"
              :key="e.line"
            >
              {{ $t('node.multi.line', { n: e.line }) }}: {{ $t('node.multi.reason.' + e.reason) }}
              <code dir="ltr">{{ e.text.length > 60 ? e.text.slice(0, 60) + '…' : e.text }}</code>
            </div>
          </v-alert>
          <v-alert
            v-if="tooMany"
            type="error"
            variant="tonal"
            density="compact"
            class="mt-2"
          >
            {{ $t('node.multi.tooMany', { n: maxNodes }) }}
          </v-alert>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="saving"
          :disabled="parsed.nodes.length === 0 || parsed.errors.length > 0 || tooMany"
          @click="save"
        >
          {{ $t('actions.add') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Data from '@/store/modules/data'
import { i18n } from '@/locales'
import { nodePayload, parseMultiAdd } from '@/types/node'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: [] }>()
const store = Data()
const text = ref('')
const insecure = ref(false)
const saving = ref(false)
// As many as the master takes in one save.
const maxNodes = 100
const placeholder = 'de-1 https://203.0.113.10:2095/panel/ TOKEN\nnl-1 http://198.51.100.7:2095 TOKEN /app/'

watch(() => props.visible, v => {
  if (v) {
    text.value = ''
    insecure.value = false
  }
})

const parsed = computed(() => parseMultiAdd(text.value, store.nodes.map(n => n.name), insecure.value))
const tooMany = computed(() => parsed.value.nodes.length > maxNodes)

// All or nothing: the master adds none of them if one fails.
const save = async () => {
  saving.value = true
  const nodes = parsed.value.nodes.map(nodePayload)
  const ok = await store.save('nodes', 'multi', nodes, undefined, i18n.global.t('node.multi.done', { n: nodes.length }))
  saving.value = false
  if (ok) emit('close')
}
</script>

<style scoped>
.node-multi :deep(textarea) {
  font-family: monospace;
  font-size: .85rem;
}
</style>
