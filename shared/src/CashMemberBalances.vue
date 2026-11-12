<script setup lang="ts">
import { ref, watch, onUnmounted } from "vue";
import { createCashApi, type CashBalance } from "./cash";
import { cashCatalog } from "./cash-catalog";
import { createCatalogI18n } from "./localization";
import { publicApiErrorText } from "./public-api-errors";
import { useOperationToast } from "./index";
const props = defineProps<{
  basePath: string;
  brandId: string;
  memberId: string;
  locale: "en" | "zh-CN";
}>();
const i18n = createCatalogI18n(cashCatalog, props.locale),
  t = i18n.t;
watch(
  () => props.locale,
  (v) => i18n.setLocale(v),
);
const balances = ref<CashBalance[] | null>(null),
  error = ref(""),
  loading = ref(false);
let generation = 0;
async function load() {
  const token = ++generation;
  loading.value = true;
  error.value = "";
  try {
    const result = await createCashApi(props.basePath, props.brandId).balances(
      props.memberId,
    );
    if (token === generation) balances.value = result;
  } catch (cause) {
    if (token === generation)
      error.value = publicApiErrorText(cause, props.locale);
  } finally {
    if (token === generation) loading.value = false;
  }
}
watch(
  () => [props.brandId, props.memberId],
  () => {
    generation++;
    balances.value = null;
    if (props.memberId) void load();
  },
  { immediate: true },
);
onUnmounted(() => generation++);
useOperationToast(error, (v) => v, "error");
</script>
<template>
  <section class="detail-section">
    <h3>{{ t("cash.title") }}</h3>
    <div v-if="balances" class="table-wrap">
      <table class="console-table console-responsive-table">
        <thead>
          <tr>
            <th>{{ t("cash.currency") }}</th>
            <th>{{ t("cash.deposited") }}</th>
            <th>{{ t("cash.withdrawable") }}</th>
            <th>{{ t("cash.reserved") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="b in balances" :key="b.currency">
            <td :data-label="t('cash.currency')">
              {{ t(`cash.${b.currency}`) }}
            </td>
            <td :data-label="t('cash.deposited')">{{ b.deposited }}</td>
            <td :data-label="t('cash.withdrawable')">{{ b.withdrawable }}</td>
            <td :data-label="t('cash.reserved')">{{ b.reserved }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
