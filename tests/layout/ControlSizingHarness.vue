<script setup lang="ts">
import { ref } from 'vue'
import SelectControl from '../../shared/src/SelectControl.vue'
const status = ref('all')
</script>
<template>
 <form class="control-demo">
  <label>Account<input aria-label="Account" value="sample_operator"></label>
  <label>Email<input type="email" aria-label="Email" value="operator@example.com"></label>
  <label>Points<input type="number" aria-label="Points" value="10000"></label>
  <label>Status<SelectControl v-model="status" label="Status" :options="[{value:'all',label:'All active and paused members'},{value:'online',label:'Online'}]" /></label>
  <div class="filters"><input aria-label="Search" placeholder="Search"></div>
  <div class="actions"><button type="button" class="button button-primary">Save changes</button><button type="button" class="button button-secondary">Cancel</button></div>
 </form>
</template>
<style scoped>
.control-demo{box-sizing:border-box;width:min(900px,100%);margin:0 auto;padding:16px;display:grid;gap:16px}.control-demo label{display:grid;gap:6px;min-width:0}.control-demo .actions{display:grid;grid-template-columns:1fr;gap:8px}
</style>
