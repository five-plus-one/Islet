<template>
  <div class="page">
    <n-h2>图片库</n-h2>
    <n-space align="center" style="margin-bottom: 14px" :wrap="false">
      <n-input v-model:value="keyword" placeholder="搜索文件名 / URL" clearable style="width: 220px" @keyup.enter="reload" />
      <n-select v-model:value="albumFilter" :options="albumOptions" placeholder="全部相册" clearable style="width: 160px" @update:value="reload" />
      <n-input v-model:value="tagFilter" placeholder="标签" clearable style="width: 120px" @keyup.enter="reload" />
      <n-button type="primary" @click="reload">筛选</n-button>
      <n-button @click="copyAll">复制全部链接</n-button>
    </n-space>

    <n-spin :show="loading">
      <div v-if="items.length" class="image-grid">
        <div v-for="item in items" :key="item.id" class="image-card">
          <img class="thumb" :src="thumbSrc(item)" loading="lazy" @click="previewItem = item" />
          <div class="meta">
            <span class="name" :title="item.original_name">{{ item.original_name }}</span>
            <span>{{ humanBytes(item.size) }} · {{ item.width }}x{{ item.height }}</span>
            <n-space size="small">
              <n-button text size="tiny" type="primary" @click="copyURL(item)">复制</n-button>
              <n-button text size="tiny" @click="editItem = item">编辑</n-button>
              <n-button text size="tiny" type="error" @click="handleRecycle(item)">删除</n-button>
            </n-space>
          </div>
        </div>
      </div>
      <n-empty v-else description="暂无图片，去上传页试试" style="margin-top: 80px" />
    </n-spin>

    <n-space justify="end" style="margin-top: 16px">
      <n-button :disabled="!items.length" @click="loadMore">加载更多</n-button>
    </n-space>

    <n-modal :show="!!previewItem" @update:show="(v: boolean) => (v || (previewItem = null))">
      <div v-if="previewItem" style="background: #fff; padding: 16px; border-radius: 12px; max-width: 90vw">
        <img :src="previewItem.url" style="max-width: 80vw; max-height: 70vh; display: block" />
        <n-space vertical style="margin-top: 10px">
          <n-text>{{ previewItem.original_name }} · {{ humanBytes(previewItem.size) }}</n-text>
          <n-input :value="previewItem.url" readonly size="small" />
          <n-space size="small">
            <n-button size="small" @click="copyFormat(previewItem, 'markdown')">Markdown</n-button>
            <n-button size="small" @click="copyFormat(previewItem, 'html')">HTML</n-button>
            <n-button size="small" @click="copyFormat(previewItem, 'bbcode')">BBCode</n-button>
            <n-button size="small" type="primary" @click="copyURL(previewItem)">直链</n-button>
          </n-space>
        </n-space>
      </div>
    </n-modal>

    <n-modal :show="!!editItem" @update:show="(v: boolean) => (v || (editItem = null))">
      <n-card v-if="editItem" style="width: 460px" title="编辑图片" :bordered="false">
        <n-form label-placement="left" label-width="80">
          <n-form-item label="名称"><n-input v-model:value="editItem.original_name" /></n-form-item>
          <n-form-item label="相册">
            <n-select v-model:value="editItem.album_id" :options="albumOptions" clearable placeholder="未分组" />
          </n-form-item>
          <n-form-item label="标签">
            <n-dynamic-tags v-model:value="editTags" />
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="editItem = null">取消</n-button>
            <n-button type="primary" @click="handleSave">保存</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  NButton, NEmpty, NH2, NInput, NModal, NCard, NForm, NFormItem, NDynamicTags,
  NSelect, NSpace, NSpin, NText, useMessage
} from 'naive-ui';
import { api, type AlbumView, type ImageItem } from '../api';

const items = ref<ImageItem[]>([]);
const loading = ref(false);
const keyword = ref('');
const tagFilter = ref('');
const albumFilter = ref<number | null>(null);
const albums = ref<AlbumView[]>([]);
const previewItem = ref<ImageItem | null>(null);
const editItem = ref<ImageItem | null>(null);
const editTags = ref<string[]>([]);
const message = useMessage();

const albumOptions = computed(() => albums.value.map((a) => ({ label: a.name, value: a.id })));

function humanBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function thumbSrc(item: ImageItem): string {
  return item.thumb_url || item.url;
}

function buildQuery(): string {
  const params = new URLSearchParams();
  if (keyword.value) params.set('keyword', keyword.value);
  if (tagFilter.value) params.set('tag', tagFilter.value);
  if (albumFilter.value) params.set('album_id', String(albumFilter.value));
  if (items.value.length) params.set('before', items.value[items.value.length - 1].created_at);
  params.set('limit', '50');
  return `?${params.toString()}`;
}

async function reload() {
  loading.value = true;
  items.value = [];
  try {
    const result = await api.images(buildQuery());
    items.value = result.items;
  } finally {
    loading.value = false;
  }
}

async function loadMore() {
  if (!items.value.length) return;
  const result = await api.images(buildQuery());
  items.value = [...items.value, ...result.items];
}

async function copyURL(item: ImageItem) {
  await navigator.clipboard.writeText(item.url);
  message.success('已复制直链');
}

function copyFormat(item: ImageItem, format: string) {
  const name = item.original_name.replace(/\]/g, '');
  const text =
    format === 'markdown'
      ? `![${name}](${item.url})`
      : format === 'html'
        ? `<img src="${item.url}" alt="${name}" />`
        : `[img]${item.url}[/img]`;
  navigator.clipboard.writeText(text);
  message.success('已复制');
}

function copyAll() {
  if (!items.value.length) return;
  navigator.clipboard.writeText(items.value.map((i) => i.url).join('\n'));
  message.success(`已复制 ${items.value.length} 条链接`);
}

async function handleRecycle(item: ImageItem) {
  await api.recycleImage(item.id);
  items.value = items.value.filter((i) => i.id !== item.id);
  message.success('已移入回收站');
}

async function handleSave() {
  if (!editItem.value) return;
  const target = editItem.value;
  await api.updateImage(target.id, {
    name: target.original_name,
    album_id: target.album_id ?? 0,
    tags: editTags.value
  });
  message.success('已保存');
  editItem.value = null;
  await reload();
}

onMounted(async () => {
  const [albumResult] = await Promise.all([api.albums(), reload()]);
  albums.value = albumResult.items;
});
</script>
