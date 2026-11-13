<script setup lang="ts">
import { useId } from "vue";
import type { CashDestination } from "./cash";
import type { CashKey } from "./cash-catalog";
const fieldId = useId();
const model = defineModel<CashDestination>({ required: true });
withDefaults(defineProps<{ t: (key: CashKey) => string; manageStatus?: boolean }>(), { manageStatus: true });
</script>
<template>
  <div class="cash-destination-fields">
    <template v-if="model.currency !== 'USDT'">
      <div class="cash-account-field">
        <div class="cash-account-label"><label :for="`${fieldId}-holder`">{{ t("cash.holder") }}</label></div><div class="cash-account-control"><input
          :id="`${fieldId}-holder`"
          v-model.trim="model.holder"
          required
          maxlength="200"
          autocomplete="off"
      /></div></div>
      <div class="cash-account-field">
        <div class="cash-account-label"><label :for="`${fieldId}-institution`">{{ t("cash.institution") }}</label></div><div class="cash-account-control"><input
          :id="`${fieldId}-institution`"
          v-model.trim="model.institution"
          required
          maxlength="200"
          autocomplete="off"
      /></div></div>
    </template>
    <div class="cash-account-field">
      <div class="cash-account-label"><label :for="`${fieldId}-number`">{{ t(model.currency === "USDT" ? "cash.address" : "cash.number") }}</label></div><div class="cash-account-control"><input
        :id="`${fieldId}-number`"
        v-model.trim="model.number"
        required
        maxlength="200"
        :data-validation-format="t('cash.addressFormat')"
        :pattern="
          model.currency === 'USDT' ? 'T[1-9A-HJ-NP-Za-km-z]{33}' : undefined
        "
        autocomplete="off"
    /></div></div>
    <label v-if="model.id && manageStatus" class="checkbox-label"
      ><input v-model="model.enabled" type="checkbox" />{{
        t("cash.enabled")
      }}</label
    >
  </div>
</template>

<style scoped>
.cash-destination-fields { display: grid; gap: 12px; min-width: 0; }
@media (max-width: 900px), (pointer: coarse) {
  .cash-destination-fields { gap: 8px; }
}
</style>
