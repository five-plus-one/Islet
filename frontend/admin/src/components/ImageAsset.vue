<script setup lang="ts">
import { ref, watch } from "vue";
import AppIcon from "./AppIcon.vue";
const props = defineProps<{
  src: string;
  fallback?: string;
  alt: string;
  loading?: "lazy" | "eager";
}>();
const source = ref(props.src),
  failed = ref(false);
watch(
  () => props.src,
  (src) => {
    source.value = src;
    failed.value = false;
  },
);
function fail() {
  if (props.fallback && source.value !== props.fallback)
    source.value = props.fallback;
  else failed.value = true;
}
</script>
<template>
  <img
    v-if="!failed"
    :src="source"
    :alt="alt"
    :loading="loading"
    @error="fail"
  /><span v-else class="image-unavailable"
    ><AppIcon name="image" :size="28" /><span>预览暂不可用</span></span
  >
</template>
<style scoped>
.image-unavailable {
  height: 100%;
  min-height: 130px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 10px;
  color: #8b9bb0;
  font-size: 12px;
}
</style>
