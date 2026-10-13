<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NSkeleton,
  type DataTableColumns,
} from "naive-ui";
import { api } from "../api";
import { errorText, humanBytes } from "../utils";
import PageHeader from "../components/PageHeader.vue";
import AppIcon from "../components/AppIcon.vue";
const router = useRouter(),
  loading = ref(true),
  error = ref("");
const overview = ref<Record<string, unknown>>({}),
  trend = ref<{ date: string; count: number; bytes: number }[]>([]),
  groups = ref<
    {
      group_id: number;
      name: string;
      quota_bytes: number;
      used_bytes: number;
      images: number;
    }[]
  >([]);
const cards = computed(() => [
  { label: "图片总数", value: overview.value.total_images, icon: "image" },
  {
    label: "图片总容量",
    value:
      overview.value.total_bytes === undefined
        ? "—"
        : humanBytes(Number(overview.value.total_bytes)),
    icon: "storage",
  },
  { label: "今日上传", value: overview.value.today_images, icon: "upload" },
  { label: "待审核用户", value: overview.value.pending_users, icon: "users" },
]);
const max = computed(() => Math.max(1, ...trend.value.map((x) => x.count)));
const columns: DataTableColumns<(typeof groups.value)[number]> = [
  { title: "分组", key: "name" },
  { title: "图片数", key: "images" },
  {
    title: "累计用量",
    key: "used_bytes",
    render: (r) => humanBytes(r.used_bytes),
  },
  {
    title: "每位成员配额",
    key: "quota_bytes",
    render: (r) => (r.quota_bytes ? humanBytes(r.quota_bytes) : "不限"),
  },
];
async function load() {
  loading.value = true;
  error.value = "";
  const results = await Promise.allSettled([
    api.overview(),
    api.trend(14),
    api.groupUsage(),
  ]);
  if (results[0].status === "fulfilled") overview.value = results[0].value;
  if (results[1].status === "fulfilled") trend.value = results[1].value.items;
  if (results[2].status === "fulfilled") groups.value = results[2].value.items;
  const failure = results.find((r) => r.status === "rejected");
  if (failure?.status === "rejected") error.value = errorText(failure.reason);
  loading.value = false;
}
onMounted(load);
</script>
<template>
  <div class="page">
    <PageHeader
      title="运行概览"
      description="了解图片增长、容量使用与待办事项。"
      eyebrow="OVERVIEW"
      ><n-button :loading="loading" @click="load"
        ><template #icon><AppIcon name="refresh" :size="17" /></template
        >刷新数据</n-button
      ></PageHeader
    ><n-alert v-if="error" type="error" style="margin-bottom: 20px"
      >部分数据加载失败：{{ error }}
      <n-button text @click="load">重试</n-button></n-alert
    >
    <div class="stat-cards">
      <div v-for="card in cards" :key="card.label" class="surface stat-card">
        <div class="stat-label">
          {{ card.label }}<AppIcon :name="card.icon" :size="18" />
        </div>
        <n-skeleton v-if="loading" height="34px" style="margin-top: 14px" />
        <div v-else class="stat-value">{{ card.value ?? "—" }}</div>
      </div>
    </div>
    <n-card :bordered="false"
      ><div class="section-head">
        <div>
          <h2 class="section-title">上传趋势</h2>
          <p class="subtle" style="margin-bottom: 0">最近 14 天 · 图片数量</p>
        </div>
        <n-button text type="primary" @click="router.push('/images?scope=all')"
          >查看图片库 →</n-button
        >
      </div>
      <n-skeleton v-if="loading" height="230px" />
      <div
        v-else-if="trend.length"
        class="chart-bars"
        role="img"
        :aria-label="
          '近14天上传' + trend.reduce((sum, x) => sum + x.count, 0) + '张图片'
        "
      >
        <div
          v-for="day in trend"
          :key="day.date"
          class="chart-day"
          :title="`${day.date}：${day.count} 张，${humanBytes(day.bytes)}`"
        >
          <div
            class="chart-bar"
            :style="{ height: Math.max(2, (day.count / max) * 180) + 'px' }"
          />
          <span>{{ day.date.slice(5) }}</span>
        </div>
      </div>
      <p v-else class="subtle">趋势数据暂时不可用</p></n-card
    ><n-card title="分组存储用量" :bordered="false" style="margin-top: 22px"
      ><n-data-table
        :columns="columns"
        :data="groups"
        :loading="loading"
        :bordered="false"
        :pagination="{ pageSize: 10 }"
        :scroll-x="600"
    /></n-card>
  </div>
</template>
