<script setup lang="ts">
import { ref, watch } from 'vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import { globalLoadingActive, globalLoadingMessage } from '../../shared/src/global-loading'
import ConfiguredSelection from '../../shared/src/ConfiguredSelection.vue'
import SelectControl from '../../shared/src/SelectControl.vue'
import type { ConfiguredSnapshot } from '../../shared/src/configured-rules'
const props = defineProps<{ snapshot: ConfiguredSnapshot; locale: 'en' | 'zh-CN' }>()
const request = ref({ play_id: props.snapshot.catalog.plays[0]!.id, plate: 'A', options: [] as string[], points: '10', multiplier: '1' })
const loadingStarts = ref(0)
watch([globalLoadingActive,globalLoadingMessage], ([active,message])=>{if(active||message)loadingStarts.value++},{flush:'sync'})
const disabled = ref(false)
const value = ref('0')
const options = Array.from({length:30}, (_, index) => ({value:String(index),label:`${index + 1} · ${props.locale === 'en' ? 'Selection with a longer label' : '较长的选项名称'}`,disabled:index === 1}))
</script>
<template>
  <main>
    <button type="button" @click="disabled = !disabled">{{ locale === 'en' ? 'Disable plate' : '禁用盘口' }}</button>
    <SelectControl v-model="value" :options="options" :label="locale === 'en' ? 'Long list' : '长列表'" />
    <ConfiguredSelection v-model="request" sale :snapshot="snapshot" :locale="locale" :disabled="disabled" available-points="80" :quick-amounts="['10','50','100']" />
    <LoadingOverlay :label="locale === 'en' ? 'Loading' : '加载'" /><output data-testid="loading-starts">{{loadingStarts}}</output>
    <output data-testid="plate-value">{{ request.plate }}</output>
  </main>
</template>
<style scoped>
main{padding:12px;box-sizing:border-box;max-width:100%}
main>button{margin-bottom:12px}
</style>
