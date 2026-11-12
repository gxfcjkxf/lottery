<script setup lang="ts">
import { ref, watch } from "vue";
import QRCode from "qrcode";
const props = defineProps<{ address: string; label: string }>();
const emit = defineEmits<{ failure: [cause: unknown] }>();
const src = ref("");
let generation = 0;
watch(
  () => props.address,
  async (address) => {
    const token = ++generation;
    src.value = "";
    try {
      const next = await QRCode.toDataURL(address, {
        width: 224,
        margin: 2,
        errorCorrectionLevel: "M",
      });
      if (token === generation) src.value = next;
    } catch (cause) {
      emit("failure", cause);
    }
  },
  { immediate: true },
);
</script>
<template>
  <img
    v-if="src"
    :src="src"
    :alt="label"
    width="224"
    height="224"
    class="cash-qr"
  />
</template>
<style scoped>
.cash-qr {
  display: block;
  max-width: 100%;
  height: auto;
  border-radius: var(--radius-sm);
  background: white;
}
</style>
