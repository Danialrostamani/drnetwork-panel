<template>
  <v-row
    v-if="orphans.length > 0"
    justify="center"
    class="mb-2"
  >
    <v-col
      cols="12"
      md="10"
    >
      <v-alert
        type="warning"
        variant="tonal"
        rounded="xl"
        :title="$t('node.orphans.title')"
      >
        <div class="mb-2">
          {{ $t('node.orphans.desc') }}
        </div>
        <div class="d-flex flex-wrap ga-2 mb-2">
          <v-chip
            v-for="o in orphans"
            :key="o.name"
            :color="picked.includes(o.name) ? 'warning' : undefined"
            :variant="picked.includes(o.name) ? 'flat' : 'outlined'"
            prepend-icon="mdi-server-off"
            @click="toggle(o.name)"
          >
            [{{ o.name }}] · {{ $t('node.orphans.count', { clients: o.clients, links: o.links }) }}
          </v-chip>
        </div>
        <div class="text-caption mb-2">
          {{ $t('node.orphans.hint') }}
        </div>
        <v-btn
          color="warning"
          variant="flat"
          prepend-icon="mdi-link-off"
          :loading="busy"
          :disabled="picked.length === 0"
          @click="remove"
        >
          {{ $t('node.orphans.remove', { n: picked.length }) }}
        </v-btn>
      </v-alert>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
// The links deleted nodes left in clients: listed by node name so they can
// be taken out of every subscription at once.
import { onMounted, ref } from 'vue'
import { push } from 'notivue'
import HttpUtils from '@/plugins/httputil'
import Data from '@/store/modules/data'
import { i18n } from '@/locales'

interface Orphan { name: string; clients: number; links: number }

const orphans = ref<Orphan[]>([])
const picked = ref<string[]>([])
const busy = ref(false)

const load = async () => {
  const msg = await HttpUtils.get<Orphan[]>('api/orphanNodeLinks')
  if (!msg.success) return
  orphans.value = msg.obj ?? []
  picked.value = orphans.value.map(o => o.name)
}

const toggle = (name: string) => {
  picked.value = picked.value.includes(name) ? picked.value.filter(n => n !== name) : [...picked.value, name]
}

const remove = async () => {
  if (picked.value.length === 0 || !window.confirm(i18n.global.t('node.orphans.confirm', { names: picked.value.join(', ') }))) return
  busy.value = true
  try {
    const msg = await HttpUtils.post<{ clients: number }>('api/removeOrphanNodeLinks', { names: JSON.stringify(picked.value) })
    if (!msg.success) return
    push.success({ message: i18n.global.t('node.orphans.done', { n: msg.obj?.clients ?? 0 }) })
    const store = Data()
    store.lastLoad = 0
    await store.loadData()
    await load()
  } finally {
    busy.value = false
  }
}

onMounted(load)
defineExpose({ load })
</script>
