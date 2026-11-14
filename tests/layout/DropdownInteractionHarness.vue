<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import SelectControl from '../../shared/src/SelectControl.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
import { globalLoadingActive, globalLoadingMessage } from '../../shared/src/global-loading'
const props = defineProps<{ surface:string; locale:'en'|'zh-CN' }>()
const value = ref('a'), disabled = ref(false), loadingStarts = ref(0)
watch([globalLoadingActive,globalLoadingMessage], ([active,message])=>{if(active||message)loadingStarts.value++},{flush:'sync'})
const options = ref([{value:'a',label:'Alpha'},{value:'b',label:'Beta',disabled:true},{value:'c',label:'Charlie'},{value:'d',label:'Delta'}])
const refresh=setInterval(()=>{options.value=options.value.map(option=>({...option}))},1000)
onBeforeUnmount(()=>clearInterval(refresh))
const labels = props.locale==='en'?['Custom selection','Disable','Outside','Loading']:['共享选择','禁用','外部按钮','加载']
</script>
<template>
  <main :class="{'management-console':surface!=='user'}">

    <div class="actions"><button class="button button-secondary" @click="disabled=!disabled">{{labels[1]}}</button><button class="button button-secondary" data-testid="outside">{{labels[2]}}</button></div>
    <label>{{labels[0]}}<SelectControl v-model="value" :label="labels[0]!" :options="options" :disabled="disabled" /></label>
    <output data-testid="custom-value">{{value}}</output><output data-testid="loading-starts">{{loadingStarts}}</output>
    <LoadingOverlay :label="labels[3]!" />
  </main>
</template>
<style scoped>
main{padding:16px;display:grid;gap:12px;min-width:0;width:100%;max-width:720px}label{display:grid;gap:6px;min-width:0}.actions{display:flex;flex-wrap:wrap;gap:8px}
</style>
