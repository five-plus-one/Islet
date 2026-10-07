<template>
  <div class="page">
    <n-h2>数据看板</n-h2>
    <div class="stat-cards">
      <n-card v-for="item in statCards" :key="item.label" size="small">
        <n-statistic :label="item.label" :value="item.value" />
      </n-card>
    </div>
    <n-card title="近 14 天上传趋势" size="small">
      <div ref="chartEl" style="height: 300px"></div>
    </n-card>
    <n-card title="分组用量" size="small" style="margin-top: 16px">
      <n-data-table :columns="groupColumns" :data="groupRows" size="small" :bordered="false" />
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, h } from 'vue';
import { NCard, NDataTable, NH2, NStatistic, type DataTableColumns } from 'naive-ui';
import { api } from '../api';

interface Overview {
  total_images?: number;
  total_bytes?: number;
  total_users?: number;
  pending_users?: number;
  today_images?: number;
  recycled_count?: number;
}

const overview = ref<Overview>({});
const trendRows = ref<{ date: string; count: number; bytes: number }[]>([]);
const groupRows = ref<{ group_id: number; name: string; quota_bytes: number; used_bytes: number; images: number }[]>([]);
const chartEl = ref<HTMLElement>();

function humanBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

const statCards = computed(() => [
  { label: '图片总数', value: overview.value.total_images ?? 0 },
  { label: '存储用量', value: humanBytes(overview.value.total_bytes ?? 0) },
  { label: '今日上传', value: overview.value.today_images ?? 0 },
  { label: '用户数', value: overview.value.total_users ?? 0 },
  { label: '待审核用户', value: overview.value.pending_users ?? 0 },
  { label: '回收站', value: overview.value.recycled_count ?? 0 }
]);

const groupColumns: DataTableColumns<{ group_id: number; name: string; quota_bytes: number; used_bytes: number; images: number }> = [
  { title: '分组', key: 'name' },
  { title: '图片数', key: 'images' },
  { title: '已用', key: 'used', render: (row) => humanBytes(row.used_bytes) },
  {
    title: '配额',
    key: 'quota',
    render: (row) => (row.quota_bytes > 0 ? humanBytes(row.quota_bytes) : '不限')
  }
];

// 简易柱状图（避免引入 ECharts 的体积；后续可替换）
function renderChart() {
  const el = chartEl.value;
  if (!el) return;
  const max = Math.max(1, ...trendRows.value.map((r) => r.count));
  const bars = trendRows.value
    .map(
      (r) =>
        `<div style="flex:1;display:flex;flex-direction:column;align-items:center;gap:4px">
          <div title="${r.date}: ${r.count}" style="width:70%;height:${Math.round((r.count / max) * 220)}px;min-height:2px;background:#18a058;border-radius:4px 4px 0 0"></div>
          <div style="font-size:10px;color:#8aa396">${r.date.slice(5)}</div>
        </div>`
    )
    .join('');
  el.innerHTML = `<div style="display:flex;align-items:flex-end;height:260px;gap:4px">${bars}</div>`;
}

onMounted(async () => {
  const [overviewData, trend, groups] = await Promise.all([api.overview(), api.trend(14), api.groupUsage()]);
  overview.value = overviewData as Overview;
  trendRows.value = trend.items;
  groupRows.value = groups.items;
  renderChart();
});
</script>
