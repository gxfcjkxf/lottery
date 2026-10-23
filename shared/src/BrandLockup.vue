<script setup lang="ts">
defineProps<{ name: string; logoText?: string; logoUrl?: string | null }>()
defineEmits<{ logoError: [event: Event] }>()
</script>
<template>
 <a class="brand-lockup"><img v-if="logoUrl" class="presentation-logo" :src="logoUrl" :alt="logoText" crossorigin="anonymous" referrerpolicy="no-referrer" @error="$emit('logoError',$event)"/><span v-else class="brand-mark">{{logoText}}</span><span><b>{{name}}</b></span></a>
</template>
