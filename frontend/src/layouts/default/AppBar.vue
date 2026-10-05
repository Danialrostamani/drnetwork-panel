<template>
  <v-app-bar :elevation="5">
    <v-icon
      v-if="isMobile"
      icon="mdi-menu"
      @click="$emit('toggleDrawer')"
    />
    <span
      v-else
      style="width: 24px"
    />
    <v-app-bar-title
      :text="$t(<string>route.name)"
      class="align-center text-center "
    />
    <!-- A stopped core is invisible from the panel otherwise, and the flag
         survives a reboot, so it has to be said on every page. -->
    <v-chip
      v-if="maintenance"
      v-tooltip="$t('setting.maintenanceOnHint')"
      color="warning"
      variant="flat"
      density="comfortable"
      prepend-icon="mdi-wrench"
      class="mr-2"
    >
      {{ $t('setting.maintenance') }}
    </v-chip>
    <!-- Whether the managed nodes answer, on every page: online / enabled. -->
    <v-chip
      v-if="health.total > 0"
      v-tooltip="$t('node.healthHint')"
      :color="health.color"
      variant="tonal"
      density="comfortable"
      prepend-icon="mdi-server-network"
      to="/nodes"
      class="mr-2"
    >
      {{ health.online }}/{{ health.total }}
    </v-chip>
    <v-menu>
      <template #activator="{ props }">
        <v-btn
          icon
          v-bind="props"
        >
          <v-icon>mdi-translate</v-icon>
        </v-btn>
      </template>
      <v-list>
        <v-list-item
          v-for="lang in languages"
          :key="lang.value"
          :active="isActiveLocale(lang.value)"
          @click="changeLocale(lang.value)"
        >
          <v-list-item-title>{{ lang.title }}</v-list-item-title>
        </v-list-item>
      </v-list>
    </v-menu>
    <v-menu>
      <template #activator="{ props }">
        <v-btn
          icon
          v-bind="props"
        >
          <v-icon>mdi-theme-light-dark</v-icon>
        </v-btn>
      </template>
      <v-list>
        <v-list-item
          v-for="th in themes"
          :key="th.value"
          :prepend-icon="th.icon"
          :active="isActiveTheme(th.value)"
          @click="changeTheme(th.value)"
        >
          <v-list-item-title>{{ $t(`theme.${th.value}`) }}</v-list-item-title>
        </v-list-item>
      </v-list>
    </v-menu>
  </v-app-bar>
</template>

<script lang="ts" setup>
import { useLocale } from 'vuetify'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { languages } from '@/locales'
import { useThemeSwitcher } from '@/composables/useThemeSwitcher'
import { computed } from 'vue'
import Data from '@/store/modules/data'
import { nodesHealth } from '@/types/node'

defineProps<{ isMobile: boolean }>()
defineEmits<{ toggleDrawer: [] }>()

const route = useRoute()
const { locale: i18nLocale } = useI18n()
const vuetifyLocale = useLocale()
const { themes, changeTheme, isActiveTheme } = useThemeSwitcher()

const changeLocale = (l: string) => {
  i18nLocale.value = l
  vuetifyLocale.current.value = l
  localStorage.setItem('locale', l)
  window.location.reload()
}
const isActiveLocale = (l: string) => i18nLocale.value === l
const maintenance = computed((): boolean => Data().maintenance)
const health = computed(() => nodesHealth(Data().nodes ?? [], Data().nodesStatus ?? {}))
</script>
