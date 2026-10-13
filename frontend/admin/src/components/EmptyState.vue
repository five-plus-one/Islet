<script setup lang="ts">
import AppIcon from "./AppIcon.vue";
withDefaults(
  defineProps<{ title?: string; description?: string; icon?: string }>(),
  { title: "这里还没有内容", icon: "image" },
);
</script>
<template>
  <div class="empty-state">
    <span class="empty-icon"><AppIcon :name="icon" :size="30" /></span>
    <h3>{{ title }}</h3>
    <p v-if="description">{{ description }}</p>
    <slot />
  </div>
</template>
