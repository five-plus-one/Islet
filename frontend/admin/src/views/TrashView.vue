<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">回收站</n-h2>
      <n-popconfirm @positive-click="handlePurgeAll">
        <template #trigger>
          <n-button type="error" :disabled="!items.length">清空回收站</n-button>
        </template>
        将彻底删除全部回收站图片（含存储文件），确认？
      </n-popconfirm>
    </n-space>

    <n-data-table :columns="columns" :data="items" :bordered="false" size="small" />
    <n-empty v-if="!items.length" description="回收站为空" style="margin-top: 80px" />
  </div>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue';
import { NButton, NDataTable, NEmpty, NH2, NPopconfirm, NSpace, useMessage, type DataTableColumns } from 'naive-ui';
import { api, type ImageItem } from '../api';

const items = ref<ImageItem[]>([]);
const message = useMessage();

const columns: DataTableColumns<ImageItem> = [
  {
    title: '预览',
    key: 'thumb',
    render: (row) => h('img', { src: row.thumb_url || row.url, style: 'width:56px;height:40px;object-fit:cover;border-radius:4px' })
  },
  { title: '名称', key: 'original_name', ellipsis: { tooltip: true } },
  { title: '所有者', key: 'username' },
  { title: '大小', key: 'size', render: (row) => `${(row.size / 1024).toFixed(1)} KB` },
  { title: '删除时间', key: 'deleted_at', render: (row) => (row.deleted_at ? new Date(row.deleted_at).toLocaleString() : '—') },
  {
    title: '操作',
    key: 'actions',
    render: (row) =>
      h(NSpace, { size: 'small' }, () => [
        h(NButton, { size: 'tiny', text: true, type: 'primary', onClick: () => handleRestore(row) }, () => '恢复'),
        h(
          NPopconfirm,
          { onPositiveClick: () => handlePurge(row) },
          {
            trigger: () => h(NButton, { size: 'tiny', text: true, type: 'error' }, () => '彻底删除'),
            default: () => '将删除存储文件，不可恢复，确认？'
          }
        )
      ])
  }
];

async function reload() {
  items.value = (await api.recycleList()).items;
}

async function handleRestore(row: ImageItem) {
  try {
    await api.restoreImage(row.id);
    message.success('已恢复');
    await reload();
  } catch (err) {
    message.error(err instanceof Error ? err.message : '恢复失败');
  }
}

async function handlePurge(row: ImageItem) {
  await api.purgeImage(row.id);
  message.success('已彻底删除');
  await reload();
}

async function handlePurgeAll() {
  const result = await api.purgeAllRecycle();
  message.success(`已清理 ${result.purged} 张`);
  await reload();
}

onMounted(reload);
</script>
