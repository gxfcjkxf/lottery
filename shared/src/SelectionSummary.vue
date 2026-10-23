<script setup lang="ts">
import { watch } from 'vue'
import { createCatalogI18n } from './localization'
import { orderTicketCatalog } from './order-ticket-catalog'
import type { RuleTicketSelection } from './rules'
const props=defineProps<{selection: RuleTicketSelection; locale:'en'|'zh'|'zh-CN'}>()
const i18n=createCatalogI18n(orderTicketCatalog,props.locale==='zh'?'zh-CN':props.locale),ui=i18n.t
watch(()=>props.locale,value=>i18n.setLocale(value==='zh'?'zh-CN':value))
</script>
<template>
  <div class="selection-summary" data-testid="selection-summary">
    <div v-if="selection.regular?.length">
      <strong>{{ ui('regular') }}</strong
      ><span>{{ selection.regular.join(" · ") }}</span>
    </div>
    <div v-if="selection.special?.length">
      <strong>{{ ui('special') }}</strong
      ><span>{{ selection.special.join(" · ") }}</span>
    </div>
    <div v-for="(values, position) in selection.digits ?? []" :key="position">
      <strong>{{
        ui("position", { position: position + 1 })
      }}</strong
      ><span>{{ values.join(" / ") }}</span>
    </div>
    <div v-if="selection.exclude?.length">
      <strong>{{ ui('exclude') }}</strong
      ><span>{{ selection.exclude.join(" · ") }}</span>
    </div>
    <div
      v-for="(values, group) in selection.attributes ?? {}"
      :key="`attribute:${group}`"
    >
      <strong>{{ ui('attribute') }} · {{ group }}</strong
      ><span>{{ values.join(" · ") }}</span>
    </div>
    <div
      v-for="(values, name) in selection.features ?? {}"
      :key="`feature:${name}`"
    >
      <strong>{{ ui('feature') }} · {{ name }}</strong
      ><span>{{ values.join(" · ") }}</span>
    </div>
  </div>
</template>
<style scoped>
.selection-summary {
  display: grid;
  gap: 0.6rem;
  padding: 0.8rem;
  background: var(--canvas);
  border-radius: var(--radius);
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
