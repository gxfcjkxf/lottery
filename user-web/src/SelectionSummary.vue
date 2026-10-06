<script setup lang="ts">
import { computed } from "vue";
import type { RuleTicketSelection } from "../../shared/src/rules";
const props = defineProps<{
  selection: RuleTicketSelection;
  locale: "en" | "zh";
}>();
const zh = computed(() => props.locale === "zh");
</script>
<template>
  <div class="selection-summary" data-testid="selection-summary">
    <div v-if="selection.regular?.length">
      <strong>{{ zh ? "普通号码" : "Regular numbers" }}</strong
      ><span>{{ selection.regular.join(" · ") }}</span>
    </div>
    <div v-if="selection.special?.length">
      <strong>{{ zh ? "特别号码" : "Special numbers" }}</strong
      ><span>{{ selection.special.join(" · ") }}</span>
    </div>
    <div v-for="(values, position) in selection.digits ?? []" :key="position">
      <strong>{{
        zh ? `位置 ${position + 1}` : `Position ${position + 1}`
      }}</strong
      ><span>{{ values.join(" / ") }}</span>
    </div>
    <div v-if="selection.exclude?.length">
      <strong>{{ zh ? "排除号码" : "Excluded numbers" }}</strong
      ><span>{{ selection.exclude.join(" · ") }}</span>
    </div>
    <div
      v-for="(values, group) in selection.attributes ?? {}"
      :key="`attribute:${group}`"
    >
      <strong>{{ zh ? "属性" : "Attribute" }} · {{ group }}</strong
      ><span>{{ values.join(" · ") }}</span>
    </div>
    <div
      v-for="(values, name) in selection.features ?? {}"
      :key="`feature:${name}`"
    >
      <strong>{{ zh ? "特征" : "Feature" }} · {{ name }}</strong
      ><span>{{ values.join(" · ") }}</span>
    </div>
  </div>
</template>
<style scoped>
.selection-summary {
  display: grid;
  gap: 0.6rem;
  padding: 0.8rem;
  background: #f0f4ef;
  border-radius: 10px;
  min-width: 0;
}
.selection-summary > div {
  display: flex;
  gap: 1rem;
  justify-content: space-between;
  flex-wrap: wrap;
  overflow-wrap: anywhere;
}
.selection-summary strong {
  font-size: 0.86rem;
}
.selection-summary span {
  font-variant-numeric: tabular-nums;
}
</style>
