<script setup lang="ts">
import { useUserI18n } from './i18n';
const ui = useUserI18n().t;
</script>

<template>
  <div class="policy-document">
    <p>{{ ui('common.policyPlaceholder') }}</p>
  </div>
</template>

<style scoped>
.policy-document { display: grid; gap: 12px; font-size: 14px; line-height: 1.7; overflow-wrap: anywhere; }
.policy-document p { margin: 0; }
</style>
