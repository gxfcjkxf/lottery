<script setup lang="ts">
import { computed, provide, ref } from 'vue'
import { setPlatformDisplaySettings } from '../../shared/src/display-settings'
import ManagementTable from '../../shared/src/ManagementTable.vue'
import DetailModal from '../../shared/src/DetailModal.vue'
import ConfiguredBetReviewTable from '../../user-web/src/ConfiguredBetReviewTable.vue'
import BettingHistoryTable from '../../shared/src/BettingHistoryTable.vue'
import { configuredDemo, type ConfiguredCatalog } from '../../shared/src/configured-rules'
import { createUserI18n, userI18nKey } from '../../user-web/src/i18n'
import catalog from '../../backend/internal/rules/config/mark_six_v2.json'
const props = defineProps<{ surface: string; locale: 'en' | 'zh-CN' }>()
setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
const i18n = createUserI18n(); i18n.setLocale(props.locale); provide(userI18nKey, i18n)
const copy = computed(() => props.locale === 'en' ? ['Period','Odds','Points','Details','Close'] : ['期号','赔率','积分','详情','关闭'])
const entries = [{id:'trial', period:'2026115', odds:'48.99', points:'40'}]
const columns = computed(() => ['period','odds','points'].map((key,index)=>({key,label:copy.value[index]!})))
const snapshot = configuredDemo(catalog as ConfiguredCatalog), play = snapshot.catalog.plays[0]!
const lines = [{play_id:play.id,options:[snapshot.catalog.domains[play.selection.domain]!.options[0]!.id],plate:'A',stake:'40',prices:{hit:'48.99'}}]
const history = [{kind:'configured' as const,id:'trial',game_id:'trial',game_name:'Mark Six',period_no:'2026115',placed_at:'2026-10-27T00:00:00Z',order:{id:'trial',game_name:'Mark Six',member_name:'Layout member',game_id:'trial',period_id:'trial',period_no:'2026115',rule_id:'trial',sales_version:1,snapshot,quote:{bet_points:'40',lines},result:null,status:'placed',placed_at:'2026-10-27T00:00:00Z',settled_at:null}}]
const dialog = ref(false)
</script>
<template>
  <main class="header-demo" :class="{ 'management-console': surface !== 'user' }">
    <section><table class="console-table" data-testid="native-table"><thead><tr><th v-for="label in copy.slice(0,3)" :key="label">{{label}}</th></tr></thead><tbody><tr><td>2026115</td><td>48.99</td><td>40</td></tr><tr><th scope="row">{{copy[2]}}</th><td>48.99</td><td>40</td></tr></tbody></table></section>
    <section class="management-console" data-testid="shared-table"><ManagementTable :entries="entries" :columns="columns" :empty="false" empty-label=""><template #period>2026115</template><template #odds>48.99</template><template #points>40</template></ManagementTable></section>
    <section role="table" class="coverage-table"><div role="row" class="coverage-row coverage-header"><span v-for="label in copy.slice(0,3)" :key="label" role="columnheader">{{label}}</span></div><div role="row" class="coverage-row"><span role="cell">2026115</span><span role="cell">48.99</span><span role="cell">40</span></div></section>
    <section v-if="surface === 'brand'" class="bucket-table"><div class="table-head"><span v-for="label in copy.slice(0,3)" :key="label">{{label}}</span></div><div class="bucket-row"><span>2026115</span><span>48.99</span><span>40</span></div></section>
    <section v-if="surface === 'user'"><BettingHistoryTable :items="history" :locale="locale" /></section>
    <button class="button button-secondary" @click="dialog = true">{{copy[3]}}</button>
    <DetailModal v-if="dialog" :title="copy[3]!" :close-label="copy[4]!" @close="dialog=false"><ConfiguredBetReviewTable :snapshot="snapshot" :lines="lines" :locale="locale" /></DetailModal>
  </main>
</template>
<style scoped>
.header-demo{width:100%;max-width:960px;margin:0 auto;padding:12px;display:grid;gap:12px;min-width:0}
.header-demo section{min-width:0;background:var(--surface)}
.header-demo table{width:100%}.header-demo .console-table{min-width:0;white-space:normal}
.bucket-table{display:grid;grid-template-columns:repeat(3,minmax(0,1fr))}.table-head,.bucket-row{display:contents}.bucket-table span{padding:8px;min-width:0;overflow-wrap:anywhere}
.coverage-row{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));padding:8px;gap:8px;overflow-wrap:anywhere}
</style>
