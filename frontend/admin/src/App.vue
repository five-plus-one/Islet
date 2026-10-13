<template>
  <n-config-provider
    :theme="null"
    :locale="zhCN"
    :date-locale="dateZhCN"
    :theme-overrides="themeOverrides"
  >
    <n-message-provider>
      <n-dialog-provider>
        <router-view />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import {
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  zhCN,
  dateZhCN,
  type GlobalThemeOverrides,
} from "naive-ui";

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#2675df",
    primaryColorHover: "#4189eb",
    primaryColorPressed: "#1b5fbc",
    borderRadius: "8px",
    textColorBase: "#172b46",
    fontFamily:
      "Inter, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
  },
};
</script>
