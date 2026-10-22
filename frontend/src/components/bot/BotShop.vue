<template>
  <v-row>
    <v-col cols="12">
      <v-alert
        type="info"
        variant="tonal"
        density="compact"
      >
        {{ $t('setting.tgBotShopHint') }}
      </v-alert>
    </v-col>
    <v-col cols="12">
      <div class="text-subtitle-1 mb-2">
        {{ $t('setting.tgBotShop') }}
      </div>
    </v-col>
    <v-col
      cols="12"
      md="6"
    >
      <v-textarea
        :model-value="shop.welcomeFa"
        :label="$t('setting.tgBotShopWelcome')"
        rows="2"
        auto-grow
        hide-details
        @update:model-value="(v: string) => patchShop({ welcomeFa: v })"
      />
    </v-col>
    <v-col
      cols="12"
      md="6"
    >
      <v-textarea
        :model-value="shop.guideFa"
        :label="$t('setting.tgBotShopGuide')"
        rows="2"
        auto-grow
        hide-details
        @update:model-value="(v: string) => patchShop({ guideFa: v })"
      />
    </v-col>
    <v-col
      cols="12"
      sm="6"
      md="4"
    >
      <v-text-field
        :model-value="shop.payCard"
        :label="$t('setting.tgBotShopPayCard')"
        hide-details
        @update:model-value="(v: string) => patchShop({ payCard: v })"
      />
    </v-col>
    <v-col
      cols="12"
      sm="6"
      md="4"
    >
      <v-text-field
        :model-value="shop.payWallet"
        :label="$t('setting.tgBotShopPayWallet')"
        hide-details
        @update:model-value="(v: string) => patchShop({ payWallet: v })"
      />
    </v-col>
    <v-col
      cols="12"
      md="4"
    >
      <v-text-field
        :model-value="shop.payHint"
        :label="$t('setting.tgBotShopPayHint')"
        hide-details
        @update:model-value="(v: string) => patchShop({ payHint: v })"
      />
    </v-col>
    <v-col
      cols="6"
      md="3"
    >
      <v-text-field
        :model-value="String(shop.trialGB)"
        type="number"
        min="0"
        :label="$t('setting.tgBotShopTrialGB')"
        hide-details
        @update:model-value="(v: string) => patchShop({ trialGB: Number(v) || 0 })"
      />
    </v-col>
    <v-col
      cols="6"
      md="3"
    >
      <v-text-field
        :model-value="String(shop.trialHours)"
        type="number"
        min="0"
        :label="$t('setting.tgBotShopTrialHours')"
        hide-details
        @update:model-value="(v: string) => patchShop({ trialHours: Number(v) || 0 })"
      />
    </v-col>
    <v-col cols="12">
      <div class="text-subtitle-2 mb-1">
        {{ $t('setting.tgBotShopRefs') }}
      </div>
      <div class="text-caption text-medium-emphasis mb-2">
        {{ $t('setting.tgBotShopRefHint') }}
      </div>
      <div
        v-for="(r, i) in shop.refs"
        :key="r.id"
        class="d-flex flex-wrap ga-2 mb-2"
      >
        <v-text-field
          :model-value="r.word"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopRefWord')"
          style="min-width: 8rem"
          @update:model-value="(v: string) => patchShopRef(i, { word: v })"
        />
        <v-text-field
          :model-value="r.group"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopRefGroup')"
          style="min-width: 8rem"
          @update:model-value="(v: string) => patchShopRef(i, { group: v })"
        />
        <v-text-field
          :model-value="r.admin"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopRefAdmin')"
          style="min-width: 8rem"
          @update:model-value="(v: string) => patchShopRef(i, { admin: v })"
        />
        <v-btn
          icon="mdi-close"
          variant="text"
          size="small"
          @click="removeShopRef(i)"
        />
      </div>
      <v-btn
        variant="tonal"
        size="small"
        @click="addShopRef"
      >
        {{ $t('setting.tgBotShopAddRef') }}
      </v-btn>
    </v-col>
    <v-col cols="12">
      <div class="text-subtitle-2 mb-2">
        {{ $t('setting.tgBotShopPlans') }}
      </div>
      <div
        v-for="(p, i) in shop.plans"
        :key="p.id"
        class="d-flex flex-wrap ga-2 mb-2"
      >
        <v-text-field
          :model-value="p.name"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopPlanName')"
          style="min-width: 8rem"
          @update:model-value="(v: string) => patchShopPlan(i, { name: v })"
        />
        <v-text-field
          :model-value="String(p.days)"
          type="number"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopPlanDays')"
          style="max-width: 6rem"
          @update:model-value="(v: string) => patchShopPlan(i, { days: Number(v) || 0 })"
        />
        <v-text-field
          :model-value="String(p.gb)"
          type="number"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopPlanGB')"
          style="max-width: 8rem"
          @update:model-value="(v: string) => patchShopPlan(i, { gb: Number(v) || 0 })"
        />
        <v-text-field
          :model-value="p.price"
          density="compact"
          hide-details
          :label="$t('setting.tgBotShopPlanPrice')"
          style="max-width: 8rem"
          @update:model-value="(v: string) => patchShopPlan(i, { price: v })"
        />
        <v-btn
          icon="mdi-close"
          variant="text"
          size="small"
          @click="removeShopPlan(i)"
        />
      </div>
      <v-btn
        variant="tonal"
        size="small"
        @click="addShopPlan"
      >
        {{ $t('setting.tgBotShopAddPlan') }}
      </v-btn>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import { useBotSettings } from '@/plugins/botSettings'

// The shop customers see: texts, payment, default plans and introducers.
// Each seller's own payment details and plans are on the sellers tab.
const { shop, patchShop, patchShopPlan, addShopPlan, removeShopPlan, patchShopRef, addShopRef, removeShopRef } = useBotSettings()
</script>
