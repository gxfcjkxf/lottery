<script setup lang="ts">
import type { CashDestination } from "./cash";
import type { CashKey } from "./cash-catalog";
const model = defineModel<CashDestination>({ required: true });
defineProps<{ t: (key: CashKey) => string }>();
</script>
<template>
  <div class="ui-form-grid">
    <label class="ui-field-row"
      ><span>{{ t("cash.channel") }}</span><input
        :value="model.currency === 'USDT' ? 'TRON/TRC20' : t('cash.bank')"
        readonly
    /></label>
    <template v-if="model.currency !== 'USDT'">
      <label class="ui-field-row"
      ><span>{{ t("cash.holder") }}</span><input
          v-model.trim="model.holder"
          required
          maxlength="200"
          autocomplete="off"
      /></label>
      <label class="ui-field-row"
      ><span>{{ t("cash.institution") }}</span><input
          v-model.trim="model.institution"
          required
          maxlength="200"
          autocomplete="off"
      /></label>
    </template>
    <label
      >{{ t(model.currency === "USDT" ? "cash.address" : "cash.number")
      }}<input
        v-model.trim="model.number"
        required
        maxlength="200"
        :data-validation-format="t('cash.addressFormat')"
        :pattern="
          model.currency === 'USDT' ? 'T[1-9A-HJ-NP-Za-km-z]{33}' : undefined
        "
        autocomplete="off"
    /></label>
    <label class="checkbox-label"
      ><input v-model="model.enabled" type="checkbox" />{{
        t("cash.enabled")
      }}</label
    >
  </div>
</template>
