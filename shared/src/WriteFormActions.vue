<script setup lang="ts">
defineProps<{submitLabel:string;busy:boolean;invalid:boolean}>()
</script>
<template><div class="form-actions ui-form-actions"><button type="submit" class="button button-primary" :disabled="busy||invalid">{{submitLabel}}</button></div></template>
