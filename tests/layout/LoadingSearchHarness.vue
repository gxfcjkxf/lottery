<script setup lang="ts">
import { ref } from 'vue'
import FilterAssociationSearch from '../../shared/src/FilterAssociationSearch.vue'
import LoadingOverlay from '../../shared/src/LoadingOverlay.vue'
const selected = ref('')
</script>
<template>
  <FilterAssociationSearch v-model="selected" brand-id="11111111-1111-4111-8111-111111111111" surface="admin" resource="commission" kind="agent" label="Agent account" locale="en" />
  <LoadingOverlay label="Loading…" />
</template>
