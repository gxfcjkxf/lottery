<script setup lang="ts">
defineProps<{submitLabel:string;cancelLabel:string;busy:boolean;frozen:boolean;invalid:boolean}>()
defineEmits<{cancel:[]}>()
</script>
<template><div class="form-actions"><button type="button" class="button button-secondary" :disabled="busy||frozen" @click="$emit('cancel')">{{cancelLabel}}</button><button type="submit" class="button button-primary" :disabled="busy||invalid">{{submitLabel}}</button></div></template>
