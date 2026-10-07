<template>
  <div class="page">
    <n-h2>个人中心</n-h2>
    <n-grid :cols="2" :x-gap="14">
      <n-gi>
        <n-card title="账号信息" size="small">
          <n-descriptions :column="1" label-placement="left" bordered size="small">
            <n-descriptions-item label="用户名">{{ info?.user.username }}</n-descriptions-item>
            <n-descriptions-item label="邮箱">{{ info?.user.email || '—' }}</n-descriptions-item>
            <n-descriptions-item label="角色">{{ roleLabel }}</n-descriptions-item>
            <n-descriptions-item label="认证方式">{{ info?.user.auth_source === 'oidc' ? 'OIDC 单点登录' : '本地账号' }}</n-descriptions-item>
            <n-descriptions-item label="所属分组">{{ info?.group.name || '—' }}</n-descriptions-item>
          </n-descriptions>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card title="配额用量" size="small">
          <n-progress
            type="dashboard"
            :percentage="percent"
            :color="percent > 90 ? '#d03050' : '#18a058'"
            style="margin: 0 auto; display: block"
          >
            <span style="font-size: 14px">{{ humanBytes(quota.used) }} / {{ quota.limit > 0 ? humanBytes(quota.limit) : '不限' }}</span>
          </n-progress>
          <n-descriptions v-if="info?.group" :column="1" label-placement="left" size="small" style="margin-top: 12px">
            <n-descriptions-item label="单文件上限">{{ (info.group.max_file_bytes ?? 0) > 0 ? humanBytes(info.group.max_file_bytes ?? 0) : '不限' }}</n-descriptions-item>
            <n-descriptions-item label="每小时上传">{{ (info.group.uploads_per_hour ?? 0) > 0 ? `${info.group.uploads_per_hour} 次` : '不限' }}</n-descriptions-item>
            <n-descriptions-item label="允许格式">{{ (info.group.allowed_mimes ?? []).join(', ') || '全部图片' }}</n-descriptions-item>
          </n-descriptions>
        </n-card>
      </n-gi>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { NCard, NDescriptions, NDescriptionsItem, NGi, NGrid, NH2, NProgress } from 'naive-ui';
import { api, type MeInfo } from '../api';

const info = ref<MeInfo | null>(null);

const quota = computed(() => info.value?.quota ?? { limit: 0, used: 0 });
const percent = computed(() => (quota.value.limit > 0 ? Math.min(100, Math.round((quota.value.used / quota.value.limit) * 100)) : 0));
const roleLabel = computed(() => ({ super_admin: '超级管理员', admin: '管理员', user: '用户' }[info.value?.user.role ?? 'user'] ?? ''));

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

onMounted(async () => {
  info.value = await api.meInfo();
});
</script>
